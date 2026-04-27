package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// databricksOAuthSSOCmd is the leaf for
// `ana connector create databricks oauth-sso` (authStrategy=oauth_sso).
//
// Shared-credential OAuth U2M (user-to-machine) — one refresh token serves
// the whole workspace. Server accepts the row in a pending-OAuth state; the
// browser handshake at `app.textql.com/auth/databricks/callback` happens
// separately and produces the refresh token. CLI users must complete that
// in a browser — the redirect URI is hard-coded and a CLI cannot receive
// the callback.
//
// Wire/UI label mismatch: the oneof variant the server accepts is
// `oauthU2m`, NOT `oauthSso`. The SSO-vs-Individual split lives on the
// envelope's `authStrategy`, not in the nested `databricksAuth` one-of.
type databricksOAuthSSOCmd struct {
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

func (c *databricksOAuthSSOCmd) Help() string {
	return "oauth-sso   Shared-token OAuth U2M auth (authStrategy=oauth_sso).\n" +
		"Usage: ana connector create databricks oauth-sso --name <n> --host <h> --http-path <p> --catalog <c> --schema <s> --client-id <id> (--client-secret <s>|--client-secret-stdin) [--port <p>]\n" +
		"Note: a human must complete the OAuth handshake in the TextQL web app you're pointed at after create — the CLI cannot receive the redirect. The success message prints the exact URL based on the active profile."
}

func (c *databricksOAuthSSOCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.clientID, "client-id", "", "Databricks OAuth app client id (required)")
	fs.StringVar(&c.clientSecret, "client-secret", "", "Databricks OAuth app client secret (discouraged; prefer --client-secret-stdin)")
	fs.BoolVar(&c.secretStdin, "client-secret-stdin", false, "read client secret from the first stdin line")
}

func (c *databricksOAuthSSOCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireNoPositionals("connector create databricks oauth-sso", args); err != nil {
		return err
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "connector create databricks oauth-sso",
		"name", "host", "http-path", "catalog", "schema", "client-id"); err != nil {
		return err
	}
	if err := requireDatabricksCommon("connector create databricks oauth-sso",
		*c.name, *c.host, *c.httpPath, *c.port, *c.catalog, *c.schema); err != nil {
		return err
	}
	if err := requireDatabricksClientID("connector create databricks oauth-sso", c.clientID); err != nil {
		return err
	}
	resolvedSecret, err := resolveSecret("client-secret", c.clientSecret, c.secretStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create databricks oauth-sso: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "DATABRICKS",
		Name:          *c.name,
		AuthStrategy:  "oauth_sso",
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
		return fmt.Errorf("connector create databricks oauth-sso: %w", err)
	}
	endpoint := c.deps.resolveEndpoint()
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\nnote: complete OAuth at %s to activate\n",
			t.ConnectorID, t.Name, t.ConnectorType, endpoint)
		return err
	}); err != nil {
		return fmt.Errorf("connector create databricks oauth-sso: %w", err)
	}
	return nil
}
