package connector

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// snowflakeOAuthIndividualCmd is the leaf for
// `ana connector create snowflake oauth-individual`
// (authStrategy=per_member_oauth).
//
// Wire shape is identical to oauth-sso — only `authStrategy` differs.
// Unlike oauth-sso, no up-front handshake is needed: each member
// authenticates lazily at their first query. The CLI just creates the row.
type snowflakeOAuthIndividualCmd struct {
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

func (c *snowflakeOAuthIndividualCmd) Help() string {
	return "oauth-individual  Per-member OAuth auth (authStrategy=per_member_oauth).\n" +
		"Usage: ana connector create snowflake oauth-individual --name <n> --locator <acct> --database <db> --oauth-client-id <id> (--oauth-client-secret <s>|--oauth-client-secret-stdin) [--warehouse <w>] [--schema <s>] [--role <r>]\n" +
		"Note: each member authenticates lazily at first query; no up-front browser step."
}

func (c *snowflakeOAuthIndividualCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.oauthClientID, "oauth-client-id", "", "Snowflake OAuth client id (required)")
	fs.StringVar(&c.oauthClientSecret, "oauth-client-secret", "", "Snowflake OAuth client secret (discouraged; prefer stdin)")
	fs.BoolVar(&c.oauthSecretStdin, "oauth-client-secret-stdin", false, "read oauth client secret from the first stdin line")
}

func (c *snowflakeOAuthIndividualCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector create snowflake oauth-individual")
	c.Flags(fs)
	cli.ApplyAncestorFlags(ctx, fs)
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if err := cli.RequireFlags(fs, "connector create snowflake oauth-individual",
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
			return cli.UsageErrf("connector create snowflake oauth-individual: --%s must not be empty", p.name)
		}
	}
	secret, err := resolveSecret("oauth-client-secret", c.oauthClientSecret, c.oauthSecretStdin, stdio.Stdin)
	if err != nil {
		return fmt.Errorf("connector create snowflake oauth-individual: %w", err)
	}

	req := createReq{Config: configEnvelope{
		ConnectorType: "SNOWFLAKE",
		Name:          *c.name,
		AuthStrategy:  "per_member_oauth",
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
		return fmt.Errorf("connector create snowflake oauth-individual: %w", err)
	}
	var typed createResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *createResp) error {
		_, err := fmt.Fprintf(w, "connectorId: %d\nname: %s\nconnectorType: %s\nnote: members authenticate lazily at first query\n",
			t.ConnectorID, t.Name, t.ConnectorType)
		return err
	}); err != nil {
		return fmt.Errorf("connector create snowflake oauth-individual: %w", err)
	}
	return nil
}
