package connector

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/textql/ana-cli/internal/cli"
)

// createCmd implements `ana connector create` — POST CreateConnector.
// v1 only supports the `postgres` dialect; other values are a usage error.
type createCmd struct{ deps Deps }

func (c *createCmd) Help() string {
	return "create   Create a new connector (postgres only in v1).\n" +
		"Usage: ana connector create --type postgres --name <name> --host <h> --port <p> --user <u> (--password-stdin|--password <p>) --database <db> [--ssl]"
}

// createReq mirrors the exact wire shape captured in the API catalog. Field
// names are protobuf camelCase; anything else is rejected server-side.
type createReq struct {
	Config configEnvelope `json:"config"`
}

// configEnvelope is also used by update; see update.go. The Postgres pointer
// so we can omit the block when no postgres flags were set (update's partial
// case).
type configEnvelope struct {
	ConnectorType string        `json:"connectorType,omitempty"`
	Name          string        `json:"name,omitempty"`
	Postgres      *postgresSpec `json:"postgres,omitempty"`
}

// postgresSpec matches the oneof leaf for the POSTGRES dialect. Port is an int
// per the catalog; sslMode is a boolean named `sslMode` (not `ssl`).
type postgresSpec struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	SSLMode  bool   `json:"sslMode,omitempty"`
}

// createResp is the `{connectorId, name, connectorType}` captured response.
type createResp struct {
	ConnectorID   int    `json:"connectorId"`
	Name          string `json:"name"`
	ConnectorType string `json:"connectorType"`
}

func (c *createCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("connector create")
	typ := fs.String("type", "", "connector type (postgres — required)")
	name := fs.String("name", "", "connector name (required)")
	host := fs.String("host", "", "database host (required)")
	port := fs.Int("port", 0, "database port (required)")
	user := fs.String("user", "", "database user (required)")
	pass := fs.String("password", "", "database password (discouraged; prefer --password-stdin)")
	passStdin := fs.Bool("password-stdin", false, "read password from the first stdin line")
	database := fs.String("database", "", "database name (required)")
	ssl := fs.Bool("ssl", false, "enable SSL/TLS")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *typ != "postgres" {
		return usageErrf("connector create: --type must be \"postgres\" (got %q)", *typ)
	}
	missing := requiredMissing(map[string]string{
		"--name":     *name,
		"--host":     *host,
		"--user":     *user,
		"--database": *database,
	})
	if *port == 0 {
		missing = append(missing, "--port")
	}
	if len(missing) > 0 {
		return usageErrf("connector create: missing required flags: %s", strings.Join(missing, ", "))
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
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed createResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("connector create: decode response: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "connectorId: %d\nname: %s\nconnectorType: %s\n",
		typed.ConnectorID, typed.Name, typed.ConnectorType)
	return nil
}

// requiredMissing collects names of string flags whose values are blank. Used
// so we can report every missing flag in one usage error rather than one at a
// time.
func requiredMissing(pairs map[string]string) []string {
	var missing []string
	for name, val := range pairs {
		if val == "" {
			missing = append(missing, name)
		}
	}
	// Deterministic order for stable test assertions.
	sortStrings(missing)
	return missing
}

// resolvePassword resolves the password from either --password-stdin (reads
// one line from r) or --password. If both are set, --password-stdin wins (it's
// the more secure channel). Neither set → usage error.
func resolvePassword(passFlag string, stdinFlag bool, r io.Reader) (string, error) {
	if stdinFlag {
		if r == nil {
			return "", errors.New("--password-stdin requires a readable stdin")
		}
		scanner := bufio.NewScanner(r)
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		// Empty stream is a usage error; the flag explicitly promised a line.
		return "", usageErrf("--password-stdin set but stdin was empty")
	}
	if passFlag == "" {
		return "", usageErrf("--password or --password-stdin is required")
	}
	return passFlag, nil
}

// sortStrings is strings.Sort-like but avoids importing sort from a utility
// file where only this tiny helper is needed. Kept private to flags.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// flagWasSet reports whether fs saw name as an explicit argument. Used by the
// update command to build a partial config from only the user-supplied flags.
func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
