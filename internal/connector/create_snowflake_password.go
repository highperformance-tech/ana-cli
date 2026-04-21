package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// snowflakePasswordCmd is the leaf for
// `ana connector create snowflake password`. Ancestor flag pointers
// (--name, --locator, --database, --warehouse, --schema, --role) come from
// newSnowflakeCreateGroup's closure.
type snowflakePasswordCmd struct {
	deps Deps

	// Ancestor-flag targets.
	name      *string
	locator   *string
	database  *string
	warehouse *string
	schema    *string
	role      *string

	// Leaf-specific flag targets.
	user      string
	password  string
	passStdin bool
}

func (c *snowflakePasswordCmd) Help() string {
	return "password   Password-based Snowflake auth (authStrategy=service_role).\n" +
		"Usage: ana connector create snowflake password --name <n> --locator <acct> --database <db> --user <u> (--password <p>|--password-stdin) [--warehouse <w>] [--schema <s>] [--role <r>]"
}

func (c *snowflakePasswordCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.user, "user", "", "Snowflake username (required)")
	fs.StringVar(&c.password, "password", "", "Snowflake password (discouraged; prefer --password-stdin)")
	fs.BoolVar(&c.passStdin, "password-stdin", false, "read password from the first stdin line")
}

func (c *snowflakePasswordCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create snowflake password")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create snowflake password",
		"name", "locator", "database", "user"); err != nil {
		return err
	}
	for _, p := range []struct {
		name, val string
	}{{"name", *c.name}, {"locator", *c.locator}, {"database", *c.database}, {"user", c.user}} {
		if p.val == "" {
			return cli.UsageErrf("connector create snowflake password: --%s must not be empty", p.name)
		}
	}
	resolvedPass, err := resolvePassword(c.password, c.passStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create snowflake password: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "SNOWFLAKE",
		Name:          *c.name,
		AuthStrategy:  "service_role",
		Snowflake: &snowflakeSpec{
			Locator:   *c.locator,
			Database:  *c.database,
			Warehouse: *c.warehouse,
			Schema:    *c.schema,
			Role:      *c.role,
			Username:  c.user,
			Password:  resolvedPass,
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create snowflake password: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create snowflake password: %w", err)
	}
	return nil
}
