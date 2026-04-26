package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// snowflakeOAuthSSOCmd is the leaf for
// `ana connector create snowflake oauth-sso`. authStrategy=oauth_sso.
//
// Server accepts CreateConnector in a pending-OAuth state — the row
// persists with just {name, locator, database, oauthClientId,
// oauthClientSecret}; the browser handshake at
// `app.textql.com/auth/snowflake/callback` happens separately and is what
// actually produces the refresh token. CLI users must complete that in a
// browser; the CLI ships as a real leaf that creates the row.
type snowflakeOAuthSSOCmd struct {
	deps Deps

	name      *string
	locator   *string
	database  *string
	warehouse *string
	schema    *string
	role      *string

	oauthClientID     string
	oauthClientSecret string
	oauthSecretStdin  bool
}

func (c *snowflakeOAuthSSOCmd) Help() string {
	return "oauth-sso  Shared-token OAuth auth (authStrategy=oauth_sso).\n" +
		"Usage: ana connector create snowflake oauth-sso --name <n> --locator <acct> --database <db> --oauth-client-id <id> (--oauth-client-secret <s>|--oauth-client-secret-stdin) [--warehouse <w>] [--schema <s>] [--role <r>]\n" +
		"Note: a human must complete the OAuth handshake in the TextQL web app you're pointed at after create — the CLI cannot receive the redirect. The success message prints the exact URL based on the active profile."
}

func (c *snowflakeOAuthSSOCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.oauthClientID, "oauth-client-id", "", "Snowflake OAuth client id (required)")
	fs.StringVar(&c.oauthClientSecret, "oauth-client-secret", "", "Snowflake OAuth client secret (discouraged; prefer stdin)")
	fs.BoolVar(&c.oauthSecretStdin, "oauth-client-secret-stdin", false, "read oauth client secret from the first stdin line")
}

func (c *snowflakeOAuthSSOCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("connector create snowflake oauth-sso: unexpected positional arguments: %v", args)
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "connector create snowflake oauth-sso",
		"name", "locator", "database", "oauth-client-id"); err != nil {
		return err
	}
	for _, p := range []struct {
		name, val string
	}{
		{"name", *c.name}, {"locator", *c.locator}, {"database", *c.database},
		{"oauth-client-id", c.oauthClientID},
	} {
		if p.val == "" {
			return cli.UsageErrf("connector create snowflake oauth-sso: --%s must not be empty", p.name)
		}
	}
	secret, err := resolveSecret("oauth-client-secret", c.oauthClientSecret, c.oauthSecretStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create snowflake oauth-sso: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "SNOWFLAKE",
		Name:          *c.name,
		AuthStrategy:  "oauth_sso",
		Snowflake: &snowflakeSpec{
			Locator:           *c.locator,
			Database:          *c.database,
			Warehouse:         *c.warehouse,
			Schema:            *c.schema,
			Role:              *c.role,
			OAuthClientID:     c.oauthClientID,
			OAuthClientSecret: secret,
		},
	}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CreateConnector", req, &raw); err != nil {
		return fmt.Errorf("connector create snowflake oauth-sso: %w", err)
	}
	endpoint := c.deps.resolveEndpoint()
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\nnote: complete OAuth at %s to activate\n",
			t.ConnectorID, t.Name, t.ConnectorType, endpoint)
		return err
	}); err != nil {
		return fmt.Errorf("connector create snowflake oauth-sso: %w", err)
	}
	return nil
}
