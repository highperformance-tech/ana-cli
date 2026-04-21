package connector

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// createCmd implements `ana connector create` — POST CreateConnector.
// v1 only supports the `postgres` dialect; other values are a usage error.
type createCmd struct{ deps Deps }

func (c *createCmd) Help() string {
	return "create   Create a new connector (postgres only in v1).\n" +
		"Usage: ana connector create --type postgres --name <name> --host <h> --port <p> --user <u> (--password-stdin|--password <p>) --database <db> [--ssl]"
}

func (c *createCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create")
	var typ string
	fs.Var(cli.EnumFlag(&typ, []string{"postgres"}), "type", "connector type (postgres — required)")
	name := fs.String("name", "", "connector name (required)")
	host := fs.String("host", "", "database host (required)")
	port := fs.Int("port", 0, "database port (required)")
	user := fs.String("user", "", "database user (required)")
	pass := fs.String("password", "", "database password (discouraged; prefer --password-stdin)")
	passStdin := fs.Bool("password-stdin", false, "read password from the first stdin line")
	database := fs.String("database", "", "database name (required)")
	ssl := fs.Bool("ssl", false, "enable SSL/TLS")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create", "type", "name", "host", "port", "user", "database"); err != nil {
		return err
	}
	// RequireFlags only checks that the flag was set, not its value. Guard
	// against explicit empties (`--name ""`) and out-of-range ports so we
	// fail fast with a local usage error rather than surfacing a server-side
	// error. Deterministic order so tests can assert which flag triggered.
	for _, p := range []struct {
		name, val string
	}{{"name", *name}, {"host", *host}, {"user", *user}, {"database", *database}} {
		if p.val == "" {
			return cli.UsageErrf("connector create: --%s must not be empty", p.name)
		}
	}
	if *port <= 0 || *port > 65535 {
		return cli.UsageErrf("connector create: --port must be in 1..65535 (got %d)", *port)
	}
	resolvedPass, err := resolvePassword(*pass, *passStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "POSTGRES",
		Name:          *name,
		Postgres: &postgresSpec{
			Host:     *host,
			Port:     *port,
			User:     *user,
			Password: resolvedPass,
			Database: *database,
			SSLMode:  *ssl,
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create: %w", err)
	}
	return nil
}

// resolvePassword resolves the password from either --password-stdin (reads
// one line from r via cli.ReadPassword, preserving every byte except the
// trailing line terminator) or --password. If both are set, --password-stdin
// wins (it's the more secure channel). Neither set → usage error. Preserving
// surrounding whitespace is intentional: a password may legitimately start or
// end with spaces/tabs, and silently trimming would cause hard-to-diagnose
// auth failures.
func resolvePassword(passFlag string, stdinFlag bool, r io.Reader) (string, error) {
	if stdinFlag {
		pass, err := cli.ReadPassword(r)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if pass == "" {
			// Empty stream is a usage error; the flag explicitly promised a line.
			return "", cli.UsageErrf("--password-stdin set but stdin was empty")
		}
		return pass, nil
	}
	if passFlag == "" {
		return "", cli.UsageErrf("--password or --password-stdin is required")
	}
	return passFlag, nil
}
