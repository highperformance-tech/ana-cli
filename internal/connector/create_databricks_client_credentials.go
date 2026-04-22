package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// databricksClientCredentialsCmd is the leaf for
// `ana connector create databricks client-credentials`. authStrategy=service_role
// (same as Access Token) — server distinguishes by populating
// `databricksAuth.clientCredentials` instead of `databricksAuth.pat`.
//
// OAuth 2.0 M2M using a Databricks Service Principal's OAuth
// clientId + clientSecret. The SP must have CAN_USE on the target SQL
// warehouse. `clientId` is a UUID and is echoed back by GetConnector;
// `clientSecret` is kept server-side.
type databricksClientCredentialsCmd struct {
	deps Deps

	name     *string
	host     *string
	httpPath *string
	port     *int
	catalog  *string
	schema   *string

	clientID     string
	clientSecret string
	secretStdin  bool
}

func (c *databricksClientCredentialsCmd) Help() string {
	return "client-credentials  OAuth M2M via Service Principal (authStrategy=service_role).\n" +
		"Usage: ana connector create databricks client-credentials --name <n> --host <h> --http-path <p> --catalog <c> --schema <s> --client-id <id> (--client-secret <s>|--client-secret-stdin) [--port <p>]"
}

func (c *databricksClientCredentialsCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.clientID, "client-id", "", "Databricks Service Principal OAuth client id / applicationId (required)")
	fs.StringVar(&c.clientSecret, "client-secret", "", "Databricks Service Principal OAuth client secret (discouraged; prefer --client-secret-stdin)")
	fs.BoolVar(&c.secretStdin, "client-secret-stdin", false, "read client secret from the first stdin line")
}

func (c *databricksClientCredentialsCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create databricks client-credentials")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create databricks client-credentials",
		"name", "host", "http-path", "catalog", "schema", "client-id"); err != nil {
		return err
	}
	if err := requireDatabricksCommon("connector create databricks client-credentials",
		*c.name, *c.host, *c.httpPath, *c.port, *c.catalog, *c.schema); err != nil {
		return err
	}
	if err := requireDatabricksClientID("connector create databricks client-credentials", c.clientID); err != nil {
		return err
	}
	resolvedSecret, err := resolveSecret("client-secret", c.clientSecret, c.secretStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create databricks client-credentials: %w", err)
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
				ClientCredentials: &databricksClientCredentialsAuth{
					ClientID:     c.clientID,
					ClientSecret: resolvedSecret,
				},
			},
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create databricks client-credentials: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create databricks client-credentials: %w", err)
	}
	return nil
}
