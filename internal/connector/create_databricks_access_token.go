package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// databricksAccessTokenCmd is the leaf for
// `ana connector create databricks access-token`. Ancestor flag pointers
// (--name, --host, --http-path, --port, --catalog, --schema) come from
// newDatabricksCreateGroup's closure.
//
// authStrategy=service_role — same value Databricks client-credentials uses.
// The server discriminates Access Token vs Client Credentials by which
// nested `databricksAuth.*` variant is populated (`pat` vs
// `clientCredentials`), NOT by authStrategy (matches Snowflake's
// password-vs-keypair discrimination pattern).
type databricksAccessTokenCmd struct {
	deps Deps

	// Ancestor-flag targets.
	name     *string
	host     *string
	httpPath *string
	port     *int
	catalog  *string
	schema   *string

	// Leaf-specific flag targets.
	token      string
	tokenStdin bool
}

func (c *databricksAccessTokenCmd) Help() string {
	return "access-token   Personal Access Token (PAT) Databricks auth (authStrategy=service_role).\n" +
		"Usage: ana connector create databricks access-token --name <n> --host <h> --http-path <p> --catalog <c> --schema <s> (--token <t>|--token-stdin) [--port <p>]"
}

func (c *databricksAccessTokenCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.token, "token", "", "Databricks personal access token (discouraged; prefer --token-stdin)")
	fs.BoolVar(&c.tokenStdin, "token-stdin", false, "read access token from the first stdin line")
}

func (c *databricksAccessTokenCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "connector create databricks access-token",
		"name", "host", "http-path", "catalog", "schema"); err != nil {
		return err
	}
	if err := requireDatabricksCommon("connector create databricks access-token",
		*c.name, *c.host, *c.httpPath, *c.port, *c.catalog, *c.schema); err != nil {
		return err
	}
	resolvedToken, err := resolveSecret("token", c.token, c.tokenStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create databricks access-token: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "DATABRICKS",
		Name:          *c.name,
		AuthStrategy:  "service_role",
		Databricks: &databricksSpec{
			Host:     *c.host,
			HTTPPath: *c.httpPath,
			Port:     *c.port,
			Catalog:  *c.catalog,
			Schema:   *c.schema,
			Auth: &databricksAuthSpec{
				Pat: &databricksPatAuth{Token: resolvedToken},
			},
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create databricks access-token: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create databricks access-token: %w", err)
	}
	return nil
}
