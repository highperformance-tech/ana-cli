package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// databricksOAuthIndividualCmd is the leaf for
// `ana connector create databricks oauth-individual`
// (authStrategy=per_member_oauth).
//
// Wire shape is identical to oauth-sso (both populate `databricksAuth.oauthU2m`)
// — only the envelope-level `authStrategy` differs. Unlike oauth-sso, no
// up-front handshake is needed: each member authenticates lazily at their
// first query. The CLI just creates the row.
type databricksOAuthIndividualCmd struct {
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

func (c *databricksOAuthIndividualCmd) Help() string {
	return "oauth-individual  Per-member OAuth U2M auth (authStrategy=per_member_oauth).\n" +
		"Usage: ana connector create databricks oauth-individual --name <n> --host <h> --http-path <p> --catalog <c> --schema <s> --client-id <id> (--client-secret <s>|--client-secret-stdin) [--port <p>]\n" +
		"Note: each member authenticates lazily at first query; no up-front browser step."
}

func (c *databricksOAuthIndividualCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.clientID, "client-id", "", "Databricks OAuth app client id (required)")
	fs.StringVar(&c.clientSecret, "client-secret", "", "Databricks OAuth app client secret (discouraged; prefer --client-secret-stdin)")
	fs.BoolVar(&c.secretStdin, "client-secret-stdin", false, "read client secret from the first stdin line")
}

func (c *databricksOAuthIndividualCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create databricks oauth-individual")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create databricks oauth-individual",
		"name", "host", "http-path", "catalog", "schema", "client-id"); err != nil {
		return err
	}
	if err := requireDatabricksCommon("connector create databricks oauth-individual",
		*c.name, *c.host, *c.httpPath, *c.port, *c.catalog, *c.schema); err != nil {
		return err
	}
	if err := requireDatabricksClientID("connector create databricks oauth-individual", c.clientID); err != nil {
		return err
	}
	resolvedSecret, err := resolveSecret("client-secret", c.clientSecret, c.secretStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create databricks oauth-individual: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "DATABRICKS",
		Name:          *c.name,
		AuthStrategy:  "per_member_oauth",
		Databricks: &databricksSpec{
			Host:     *c.host,
			HTTPPath: *c.httpPath,
			Port:     *c.port,
			Catalog:  *c.catalog,
			Schema:   *c.schema,
			Auth: &databricksAuthSpec{
				OAuthU2M: &databricksOAuthU2MAuth{
					ClientID:     c.clientID,
					ClientSecret: resolvedSecret,
				},
			},
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create databricks oauth-individual: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\nnote: members authenticate lazily at first query\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create databricks oauth-individual: %w", err)
	}
	return nil
}
