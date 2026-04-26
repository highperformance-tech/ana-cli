package auth

import (
	"context"
	"flag"
	"fmt"
	"io"

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

type listApiKeysResp struct {
	APIKeys []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		LastUsedAt string `json:"lastUsedAt"`
		CreatedAt  string `json:"createdAt"`
	} `json:"apiKeys"`
}

func (c *keysListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("auth keys list: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListApiKeys", struct{}{}, &raw); err != nil {
		return fmt.Errorf("auth keys list: %w", translateErr(err))
	}
	var typed listApiKeysResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listApiKeysResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME\tLAST USED")
		for _, k := range t.APIKeys {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", k.ID, k.Name, cli.DashIfEmpty(k.LastUsedAt))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("auth keys list: %w", err)
	}
	return nil
}

// ---- create ----

type keysCreateCmd struct {
	deps Deps
	name string
	sa   string
}

func (c *keysCreateCmd) Help() string {
	return "keys create   Create an API key and print the plaintext token ONCE.\n" +
		"Usage: ana auth keys create --name <name> [--service-account <id>]\n" +
		"\n" +
		"--service-account takes a memberId, which is per-org: the same service account\n" +
		"in a different org has a different memberId. Get the right id from\n" +
		"`ana auth service-accounts list` while on the target org's profile."
}

func (c *keysCreateCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.name, "name", "", "human-readable name (required)")
	fs.StringVar(&c.sa, "service-account", "", "optional service account member id")
}

type createApiKeyReq struct {
	Name             string `json:"name"`
	ServiceAccountID string `json:"serviceAccountId,omitempty"`
}

type createApiKeyResp struct {
	APIKey struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"apiKey"`
	APIKeyHash string `json:"apiKeyHash"`
}

func (c *keysCreateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("auth keys create: unexpected positional arguments: %v", args)
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "auth keys create", "name"); err != nil {
		return err
	}
	if c.name == "" {
		return cli.UsageErrf("auth keys create: --name must not be empty")
	}

	req := createApiKeyReq{Name: c.name, ServiceAccountID: c.sa}
	var resp createApiKeyResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/CreateApiKey", req, &resp); err != nil {
		return fmt.Errorf("auth keys create: %w", translateErr(err))
	}
	emitPlaintextToken(stdio, resp.APIKeyHash)
	return nil
}

// emitPlaintextToken prints the one-shot plaintext token to stdout with a
// stderr reminder. Shared by `keys create` and `keys rotate` since both
// endpoints return the plaintext exactly once.
func emitPlaintextToken(stdio cli.IO, token string) {
	fmt.Fprintln(stdio.Stderr, "# store this token; it will not be shown again")
	fmt.Fprintln(stdio.Stdout, token)
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

func (c *keysRotateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) > 1 {
		return cli.UsageErrf("auth keys rotate: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("auth keys rotate", args)
	if err != nil {
		return err
	}
	req := rotateApiKeyReq{APIKeyID: id}
	var resp createApiKeyResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/RotateApiKey", req, &resp); err != nil {
		return fmt.Errorf("auth keys rotate: %w", translateErr(err))
	}
	emitPlaintextToken(stdio, resp.APIKeyHash)
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
	if len(args) > 1 {
		return cli.UsageErrf("auth keys revoke: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("auth keys revoke", args)
	if err != nil {
		return err
	}
	req := revokeApiKeyReq{APIKeyID: id}
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/RevokeApiKey", req, nil); err != nil {
		return fmt.Errorf("auth keys revoke: %w", translateErr(err))
	}
	fmt.Fprintf(stdio.Stdout, "revoked %s\n", id)
	return nil
}
