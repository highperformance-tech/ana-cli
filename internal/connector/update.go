package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// updateCmd implements `ana connector update <id>`. Per captured quirk,
// UpdateConnector takes `connectorId` at the top level (NOT nested in
// config). Only the fields the user set on the CLI get sent — we iterate
// flag.Visit (via cli.FlagWasSet on the resolver-parsed FlagSet) to keep the
// partial config minimal.
type updateCmd struct {
	deps Deps

	typ       string
	name      string
	host      string
	port      int
	user      string
	pass      string
	passStdin bool
	database  string
	ssl       bool
}

func (c *updateCmd) Help() string {
	return "update   Update a connector's fields (only supplied flags are sent).\n" +
		"Usage: ana connector update <id> [--type postgres] [--name ...] [--host ...] [--port ...] [--user ...] [--database ...] [--password ...|--password-stdin] [--ssl]"
}

func (c *updateCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.typ, "type", "", "connector type (postgres)")
	fs.StringVar(&c.name, "name", "", "new name")
	fs.StringVar(&c.host, "host", "", "new host")
	fs.IntVar(&c.port, "port", 0, "new port")
	fs.StringVar(&c.user, "user", "", "new user")
	fs.StringVar(&c.pass, "password", "", "new password (discouraged)")
	fs.BoolVar(&c.passStdin, "password-stdin", false, "read new password from stdin")
	fs.StringVar(&c.database, "database", "", "new database")
	fs.BoolVar(&c.ssl, "ssl", false, "enable SSL/TLS")
}

func (c *updateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.FlagSetFrom(ctx)
	if len(args) != 1 {
		return cli.UsageErrf("connector update: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector update", args)
	if err != nil {
		return err
	}

	if cli.FlagWasSet(fs, "type") && c.typ != "postgres" {
		return cli.UsageErrf("connector update: --type must be \"postgres\" (got %q)", c.typ)
	}

	dialectTouched := cli.FlagWasSet(fs, "host") || cli.FlagWasSet(fs, "port") ||
		cli.FlagWasSet(fs, "user") || cli.FlagWasSet(fs, "database") ||
		cli.FlagWasSet(fs, "ssl") || cli.FlagWasSet(fs, "password") ||
		cli.FlagWasSet(fs, "password-stdin")

	if !cli.FlagWasSet(fs, "name") && !cli.FlagWasSet(fs, "type") && !dialectTouched {
		return cli.UsageErrf("connector update: at least one field flag is required")
	}

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
		cfg.Name = c.name
	}
	if cli.FlagWasSet(fs, "host") {
		pg.Host = c.host
	}
	if cli.FlagWasSet(fs, "port") {
		pg.Port = c.port
	}
	if cli.FlagWasSet(fs, "user") {
		pg.User = c.user
	}
	if cli.FlagWasSet(fs, "database") {
		pg.Database = c.database
	}
	if cli.FlagWasSet(fs, "ssl") {
		pg.SSLMode = c.ssl
	}
	if cli.FlagWasSet(fs, "password") || cli.FlagWasSet(fs, "password-stdin") {
		resolved, err := resolveSecret("password", c.pass, c.passStdin, stdio.Stdin)
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
