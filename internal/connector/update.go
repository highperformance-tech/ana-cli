package connector

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// updateCmd implements `ana connector update <id>`. Per captured quirk,
// UpdateConnector takes `connectorId` at the top level (NOT nested in config).
// Only the fields the user set on the CLI get sent — we iterate flag.Visit to
// keep the partial config minimal.
type updateCmd struct{ deps Deps }

func (c *updateCmd) Help() string {
	return "update   Update a connector's fields (only supplied flags are sent).\n" +
		"Usage: ana connector update <id> [--type postgres] [--name ...] [--host ...] [--port ...] [--user ...] [--database ...] [--password ...|--password-stdin] [--ssl]"
}

// updateReq's `connectorId` MUST sit at the top level — putting it inside
// config returns 500 "could not find connector" (captured regression).
type updateReq struct {
	ConnectorID int            `json:"connectorId"`
	Config      configEnvelope `json:"config"`
}

func (c *updateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("connector update")
	typ := fs.String("type", "", "connector type (postgres)")
	name := fs.String("name", "", "new name")
	host := fs.String("host", "", "new host")
	port := fs.Int("port", 0, "new port")
	user := fs.String("user", "", "new user")
	pass := fs.String("password", "", "new password (discouraged)")
	passStdin := fs.Bool("password-stdin", false, "read new password from stdin")
	database := fs.String("database", "", "new database")
	ssl := fs.Bool("ssl", false, "enable SSL/TLS")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrf("connector update: <id> positional argument required")
	}
	id, err := atoiID("connector update", fs.Arg(0))
	if err != nil {
		return err
	}

	if flagWasSet(fs, "type") && *typ != "postgres" {
		return usageErrf("connector update: --type must be \"postgres\" (got %q)", *typ)
	}

	// Assemble the partial config from only the user-supplied flags.
	cfg := configEnvelope{}
	// If any dialect field was touched, we must include connectorType so the
	// server knows which oneof to interpret. The catalog shows the full body
	// includes connectorType alongside the dialect block.
	dialectTouched := false
	pg := &postgresSpec{}
	if flagWasSet(fs, "host") {
		pg.Host = *host
		dialectTouched = true
	}
	if flagWasSet(fs, "port") {
		pg.Port = *port
		dialectTouched = true
	}
	if flagWasSet(fs, "user") {
		pg.User = *user
		dialectTouched = true
	}
	if flagWasSet(fs, "database") {
		pg.Database = *database
		dialectTouched = true
	}
	if flagWasSet(fs, "ssl") {
		pg.SSLMode = *ssl
		dialectTouched = true
	}
	// Password can come from either flag or stdin; --password-stdin implies
	// the user wants to change it.
	if flagWasSet(fs, "password") || flagWasSet(fs, "password-stdin") {
		resolved, err := resolvePassword(*pass, *passStdin, stdio.Stdin)
		if err != nil {
			return fmt.Errorf("connector update: %w", err)
		}
		pg.Password = resolved
		dialectTouched = true
	}

	if flagWasSet(fs, "name") {
		cfg.Name = *name
	}
	if flagWasSet(fs, "type") || dialectTouched {
		cfg.ConnectorType = "POSTGRES"
	}
	if dialectTouched {
		cfg.Postgres = pg
	}

	// Reject no-op updates — sending an empty config would clobber the server
	// record's name/type in some implementations and is almost certainly a
	// user error.
	if !flagWasSet(fs, "name") && !flagWasSet(fs, "type") && !dialectTouched {
		return usageErrf("connector update: at least one field flag is required")
	}

	req := updateReq{ConnectorID: id, Config: cfg}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/UpdateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector update: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	if conn, ok := raw["connector"].(map[string]any); ok {
		return renderTwoCol(stdio.Stdout, conn)
	}
	return writeJSON(stdio.Stdout, raw)
}
