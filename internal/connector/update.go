package connector

import (
	"context"
	"fmt"
	"io"

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

func (c *updateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector update")
	typ := fs.String("type", "", "connector type (postgres)")
	name := fs.String("name", "", "new name")
	host := fs.String("host", "", "new host")
	port := fs.Int("port", 0, "new port")
	user := fs.String("user", "", "new user")
	pass := fs.String("password", "", "new password (discouraged)")
	passStdin := fs.Bool("password-stdin", false, "read new password from stdin")
	database := fs.String("database", "", "new database")
	ssl := fs.Bool("ssl", false, "enable SSL/TLS")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.UsageErrf("connector update: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector update", fs.Args())
	if err != nil {
		return err
	}

	if cli.FlagWasSet(fs, "type") && *typ != "postgres" {
		return cli.UsageErrf("connector update: --type must be \"postgres\" (got %q)", *typ)
	}

	// Track which dialect-level flags the user explicitly set; those override
	// the pre-fetched baseline below.
	dialectTouched := cli.FlagWasSet(fs, "host") || cli.FlagWasSet(fs, "port") ||
		cli.FlagWasSet(fs, "user") || cli.FlagWasSet(fs, "database") ||
		cli.FlagWasSet(fs, "ssl") || cli.FlagWasSet(fs, "password") ||
		cli.FlagWasSet(fs, "password-stdin")

	if !cli.FlagWasSet(fs, "name") && !cli.FlagWasSet(fs, "type") && !dialectTouched {
		return cli.UsageErrf("connector update: at least one field flag is required")
	}

	// The server rejects partial updates: if connectorType is POSTGRES the
	// postgres block must accompany it ("postgres metadata missing"), and a
	// missing connectorType fails with "CONNECTOR_TYPE_UNSPECIFIED". So we
	// always pre-fetch the current connector and merge the user's flag
	// overrides on top — a rename or single-field tweak produces a valid
	// full-spec update without forcing the user to re-type every field.
	var curr getConnectorResp
	if err := c.deps.Unary(ctx, servicePath+"/GetConnector", map[string]any{"connectorId": id}, &curr); err != nil {
		return fmt.Errorf("connector update: fetch current: %w", err)
	}

	cfg := configEnvelope{
		ConnectorType: curr.Connector.ConnectorType,
		Name:          curr.Connector.Name,
	}
	pg := curr.Connector.PostgresMetadata
	if cli.FlagWasSet(fs, "type") {
		cfg.ConnectorType = "POSTGRES"
	}
	if cli.FlagWasSet(fs, "name") {
		cfg.Name = *name
	}
	if cli.FlagWasSet(fs, "host") {
		pg.Host = *host
	}
	if cli.FlagWasSet(fs, "port") {
		pg.Port = *port
	}
	if cli.FlagWasSet(fs, "user") {
		pg.User = *user
	}
	if cli.FlagWasSet(fs, "database") {
		pg.Database = *database
	}
	if cli.FlagWasSet(fs, "ssl") {
		pg.SSLMode = *ssl
	}
	// Password isn't returned by GetConnector (server-side secret). If the
	// user didn't touch --password{,-stdin}, leave pg.Password empty and the
	// server keeps the existing secret. Otherwise resolve and overlay.
	if cli.FlagWasSet(fs, "password") || cli.FlagWasSet(fs, "password-stdin") {
		resolved, err := resolvePassword(*pass, *passStdin, stdio.Stdin)
		if err != nil {
			return fmt.Errorf("connector update: %w", err)
		}
		pg.Password = resolved
	}
	cfg.Postgres = &pg

	req := updateReq{ConnectorID: id, Config: cfg}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/UpdateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector update: %w", err)
	}
	var typed getResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *getResp) error {
		if t.Connector == nil {
			return cli.WriteJSON(w, raw)
		}
		return cli.RenderTwoCol(w, t.Connector)
	}); err != nil {
		return fmt.Errorf("connector update: %w", err)
	}
	return nil
}
