package auth

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newKeysGroup wires up the `auth keys` verb family: list, create, rotate,
// revoke. The nested group lets root dispatch a three-token path like
// `ana auth keys list` without special handling.
func newKeysGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage API keys.",
		Children: map[string]cli.Command{
			"list":   &keysListCmd{deps: deps},
			"create": &keysCreateCmd{deps: deps},
			"rotate": &keysRotateCmd{deps: deps},
			"revoke": &keysRevokeCmd{deps: deps},
		},
	}
}

// ---- list ----

type keysListCmd struct{ deps Deps }

func (c *keysListCmd) Help() string {
	return "keys list   List API keys (table by default, --json for raw JSON).\n" +
		"Usage: ana auth keys list"
}

// listApiKeysResp is the shape we care about from rbac.RBACService/ListApiKeys.
// The API also returns fields we don't render (assumedRoles, status, ...); the
// decoder silently drops them. `Id` maps to response.id per the catalog.
type listApiKeysResp struct {
	APIKeys []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		LastUsedAt string `json:"lastUsedAt"`
		CreatedAt  string `json:"createdAt"`
	} `json:"apiKeys"`
}

// Run issues ListApiKeys, then either dumps raw JSON or prints a fixed-width
// ID/NAME/LAST USED table. LastUsedAt is not always present in the captured
// payload; empty strings render as "-" to keep the column aligned.
func (c *keysListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth keys list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListApiKeys", struct{}{}, &raw); err != nil {
		return fmt.Errorf("auth keys list: %w", translateErr(err))
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	// Narrow the map back into our typed shape for table rendering.
	var typed listApiKeysResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("auth keys list: decode response: %w", err)
	}
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "ID\tNAME\tLAST USED")
	for _, k := range typed.APIKeys {
		last := k.LastUsedAt
		if last == "" {
			last = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", k.ID, k.Name, last)
	}
	return tw.Flush()
}

// ---- create ----

type keysCreateCmd struct{ deps Deps }

func (c *keysCreateCmd) Help() string {
	return "keys create   Create an API key and print the plaintext token ONCE.\n" +
		"Usage: ana auth keys create --name <name> [--service-account <id>]\n" +
		"\n" +
		"--service-account takes a memberId, which is per-org: the same service account\n" +
		"in a different org has a different memberId. Get the right id from\n" +
		"`ana auth service-accounts list` while on the target org's profile."
}

// createApiKeyReq uses the exact camelCase names the server requires (see
// POST_...CreateApiKey.json). The `omitempty` on ServiceAccountID means the
// field vanishes from the JSON when the flag wasn't supplied — matching the
// capture sample, which doesn't include it.
type createApiKeyReq struct {
	Name             string `json:"name"`
	ServiceAccountID string `json:"serviceAccountId,omitempty"`
}

// createApiKeyResp omits fields we don't need to surface. apiKeyHash is the
// one-time plaintext; the nested apiKey.id/name is what list would show.
type createApiKeyResp struct {
	APIKey struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"apiKey"`
	APIKeyHash string `json:"apiKeyHash"`
}

// Run parses flags, asserts --name, issues the RPC, and prints the plaintext
// token to stdout with a stderr reminder that it's one-shot.
func (c *keysCreateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth keys create")
	name := fs.String("name", "", "human-readable name (required)")
	sa := fs.String("service-account", "", "optional service account member id")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if *name == "" {
		return cli.UsageErrf("auth keys create: --name is required")
	}

	req := createApiKeyReq{Name: *name, ServiceAccountID: *sa}
	var resp createApiKeyResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/CreateApiKey", req, &resp); err != nil {
		return fmt.Errorf("auth keys create: %w", translateErr(err))
	}
	fmt.Fprintln(stdio.Stderr, "# store this token; it will not be shown again")
	fmt.Fprintln(stdio.Stdout, resp.APIKeyHash)
	return nil
}

// ---- rotate ----

type keysRotateCmd struct{ deps Deps }

func (c *keysRotateCmd) Help() string {
	return "keys rotate   Rotate an API key (old key revoked, new plaintext printed).\n" +
		"Usage: ana auth keys rotate <id>"
}

type rotateApiKeyReq struct {
	APIKeyID string `json:"apiKeyId"`
}

// Run requires exactly one positional (the key id). Response re-uses the same
// shape as create so we reuse createApiKeyResp for decoding.
func (c *keysRotateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth keys rotate")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	// Reject extra trailing positionals explicitly: cli.RequireStringID only
	// inspects args[0], so without this guard `keys rotate id extra` would
	// silently accept and rotate `id`, masking a typo.
	if fs.NArg() > 1 {
		return cli.UsageErrf("auth keys rotate: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("auth keys rotate", fs.Args())
	if err != nil {
		return err
	}
	req := rotateApiKeyReq{APIKeyID: id}
	var resp createApiKeyResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/RotateApiKey", req, &resp); err != nil {
		return fmt.Errorf("auth keys rotate: %w", translateErr(err))
	}
	fmt.Fprintln(stdio.Stderr, "# store this token; it will not be shown again")
	fmt.Fprintln(stdio.Stdout, resp.APIKeyHash)
	return nil
}

// ---- revoke ----

type keysRevokeCmd struct{ deps Deps }

func (c *keysRevokeCmd) Help() string {
	return "keys revoke   Revoke an API key by id.\n" +
		"Usage: ana auth keys revoke <id>"
}

type revokeApiKeyReq struct {
	APIKeyID string `json:"apiKeyId"`
}

func (c *keysRevokeCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth keys revoke")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	// Revoke is destructive; extra positionals almost certainly indicate a
	// user typo. Reject rather than silently targeting only args[0].
	if fs.NArg() > 1 {
		return cli.UsageErrf("auth keys revoke: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("auth keys revoke", fs.Args())
	if err != nil {
		return err
	}
	req := revokeApiKeyReq{APIKeyID: id}
	// Response body is `{success: true}` — we don't need it; pass nil to
	// signal "decode nothing" to the transport layer.
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/RevokeApiKey", req, nil); err != nil {
		return fmt.Errorf("auth keys revoke: %w", translateErr(err))
	}
	fmt.Fprintf(stdio.Stdout, "revoked %s\n", id)
	return nil
}
