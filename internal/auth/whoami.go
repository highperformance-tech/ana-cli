package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// whoamiCmd prints the current member's email (or full JSON under --json).
// Path: /rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember.
type whoamiCmd struct{ deps Deps }

func (c *whoamiCmd) Help() string {
	return "whoami   Print the authenticated member's email (or full JSON with --json).\n" +
		"Usage: ana auth whoami"
}

// getMemberResp mirrors the fields we care about in the
// PublicAuthService.GetMember response. Unknown fields are ignored by the
// standard library decoder, so adding future fields won't break us.
type getMemberResp struct {
	Member struct {
		MemberID     string `json:"memberId"`
		EmailAddress string `json:"emailAddress"`
		Name         string `json:"name"`
		OrgID        string `json:"orgId"`
		Role         string `json:"role"`
	} `json:"member"`
}

// Run refuses to call Unary when no token is set (surfacing ErrNotLoggedIn
// before any network attempt), issues the RPC otherwise, and prints either
// the email or the full response JSON depending on --json.
func (c *whoamiCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth whoami")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth whoami: load config: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("auth whoami: %w", ErrNotLoggedIn)
	}

	// Keep the decoded response around even for the --json branch so we emit
	// the re-encoded payload rather than the raw wire bytes, which keeps the
	// output stable even if the server adds fields we don't model.
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember", struct{}{}, &raw); err != nil {
		return fmt.Errorf("auth whoami: %w", translateErr(err))
	}

	global := cli.GlobalFrom(ctx)
	if global.JSON {
		enc := json.NewEncoder(stdio.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(raw); err != nil {
			return fmt.Errorf("auth whoami: encode response: %w", err)
		}
		return nil
	}

	// Pull the email out of the generic map; defensive because a partial
	// response (no `member` field) shouldn't panic.
	email := ""
	if m, ok := raw["member"].(map[string]any); ok {
		if e, ok := m["emailAddress"].(string); ok {
			email = e
		}
	}
	if email == "" {
		return fmt.Errorf("auth whoami: response missing member.emailAddress")
	}
	fmt.Fprintln(stdio.Stdout, email)
	return nil
}
