package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newPostgresCreateGroup returns the Postgres create-dialect Group. Flags
// common to every Postgres auth-mode leaf (`--name`, `--ssl`) are declared
// on the Group's persistent Flags closure; each auth-mode leaf (today only
// `password`) declares its own dialect-specific flags via Flagger. The
// resolver merges them onto the leaf's parsed FlagSet automatically.
//
// The shared `name`/`ssl` vars live in this builder's closure: the CLI is
// single-shot so one set of mutable targets per-Group is fine.
func newPostgresCreateGroup(deps Deps) *cli.Group {
	var name string
	var ssl bool
	return &cli.Group{
		Summary: "Create a Postgres connector. Pick an auth mode.",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "connector name (required)")
			fs.BoolVar(&ssl, "ssl", false, "enable SSL/TLS")
		},
		Children: map[string]cli.Command{
			"password": &postgresPasswordCmd{deps: deps, name: &name, ssl: &ssl},
		},
	}
}

// postgresPasswordCmd is the leaf for `ana connector create postgres password`.
// name/ssl are pointers into the parent Group's Flags closure state — the
// Group's persistent flag registrar binds --name/--ssl on the merged FlagSet
// to those addresses, so reading them after parse is equivalent to reading
// any other flag target.
type postgresPasswordCmd struct {
	deps Deps
	name *string
	ssl  *bool

	host      string
	port      int
	user      string
	database  string
	password  string
	passStdin bool
}

func (c *postgresPasswordCmd) Help() string {
	return "password   Password-based Postgres auth.\n" +
		"Usage: ana connector create postgres password --name <n> --host <h> --port <p> --user <u> --database <db> (--password <p>|--password-stdin) [--ssl]"
}

func (c *postgresPasswordCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.host, "host", "", "database host (required)")
	fs.IntVar(&c.port, "port", 0, "database port (required)")
	fs.StringVar(&c.user, "user", "", "database user (required)")
	fs.StringVar(&c.database, "database", "", "database name (required)")
	fs.StringVar(&c.password, "password", "", "database password (discouraged; prefer --password-stdin)")
	fs.BoolVar(&c.passStdin, "password-stdin", false, "read password from the first stdin line")
}

func (c *postgresPasswordCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireNoPositionals("connector create postgres password", args); err != nil {
		return err
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "connector create postgres password",
		"name", "host", "port", "user", "database"); err != nil {
		return err
	}
	for _, p := range []struct {
		name, val string
	}{{"name", *c.name}, {"host", c.host}, {"user", c.user}, {"database", c.database}} {
		if p.val == "" {
			return cli.UsageErrf("connector create postgres password: --%s must not be empty", p.name)
		}
	}
	if c.port <= 0 || c.port > 65535 {
		return cli.UsageErrf("connector create postgres password: --port must be in 1..65535 (got %d)", c.port)
	}
	resolvedPass, err := resolveSecret("password", c.password, c.passStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create postgres password: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "POSTGRES",
		Name:          *c.name,
		Postgres: &postgresSpec{
			Host:     c.host,
			Port:     c.port,
			User:     c.user,
			Password: resolvedPass,
			Database: c.database,
			SSLMode:  *c.ssl,
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create postgres password: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create postgres password: %w", err)
	}
	return nil
}
