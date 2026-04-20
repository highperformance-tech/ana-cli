package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// whoamiCmd prints the authenticated member's identity — email, active org
// name, orgId, and role — as a tabwriter-formatted key/value list (or the
// combined raw JSON of both RPCs under --json). It fans out GetMember and
// GetOrganization in parallel because they are independent RPCs and serial
// issuance would needlessly double command latency.
//
// Paths:
//
//	/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember
//	/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization
type whoamiCmd struct{ deps Deps }

func (c *whoamiCmd) Help() string {
	return "whoami   Print email, org, and role (tabwriter) or both raw responses with --json.\n" +
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

// getOrganizationResp narrows the fields we render from GetOrganization. The
// full payload includes theme/toolRestrictions/etc.; the decoder silently
// drops them.
type getOrganizationResp struct {
	Organization struct {
		OrgID            string `json:"orgId"`
		OrganizationName string `json:"organizationName"`
	} `json:"organization"`
}

// Run refuses to call Unary when no token is set (surfacing ErrNotLoggedIn
// before any network attempt), fans out GetMember and GetOrganization in
// parallel, then prints either the tabwriter summary or a wrapper object
// containing both raw responses under --json.
func (c *whoamiCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("auth whoami")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth whoami: load config: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("auth whoami: %w", ErrNotLoggedIn)
	}

	// The first RPC to fail cancels its sibling so the loser is interrupted at
	// its next ctx check instead of burning its full request budget.
	fanCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		memberRaw map[string]any
		orgRaw    map[string]any
		memberErr error
		orgErr    error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if e := c.deps.Unary(fanCtx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember", struct{}{}, &memberRaw); e != nil {
			memberErr = translateErr(e)
			cancel()
		}
	}()
	go func() {
		defer wg.Done()
		if e := c.deps.Unary(fanCtx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization", struct{}{}, &orgRaw); e != nil {
			orgErr = translateErr(e)
			cancel()
		}
	}()
	wg.Wait()
	// errors.Join(nil, nil) returns nil; a single-error case wraps in a
	// joinError whose .Error() is just the inner message. Is/As traverse through.
	if err := errors.Join(memberErr, orgErr); err != nil {
		return fmt.Errorf("auth whoami: %w", err)
	}

	if cli.GlobalFrom(ctx).JSON {
		// Wrap both raw maps so neither response is lost. This matches the
		// "preserve what the server sent" contract the other --json paths
		// follow (writeJSON uses the same 2-space indent).
		return cli.WriteJSON(stdio.Stdout, map[string]any{
			"member":       memberRaw,
			"organization": orgRaw,
		})
	}

	var member getMemberResp
	if err := cli.Remarshal(memberRaw, &member); err != nil {
		return fmt.Errorf("auth whoami: decode response: %w", err)
	}
	// Org is decoded lazily — a missing organizationName is tolerated (org is
	// secondary to the "who am I" identity claim), but a missing email still
	// errors because it's the primary assertion of this command.
	var org getOrganizationResp
	if err := cli.Remarshal(orgRaw, &org); err != nil {
		return fmt.Errorf("auth whoami: decode response: %w", err)
	}
	if member.Member.EmailAddress == "" {
		return fmt.Errorf("auth whoami: response missing member.emailAddress")
	}

	tw := cli.NewTableWriter(stdio.Stdout)
	// Keys mirror the `org show` aesthetic: camelCase wire names where they
	// exist (orgId), human-friendly shortenings where brevity helps
	// (organization, not organizationName).
	fmt.Fprintf(tw, "email\t%s\n", member.Member.EmailAddress)
	fmt.Fprintf(tw, "organization\t%s\n", org.Organization.OrganizationName)
	fmt.Fprintf(tw, "orgId\t%s\n", member.Member.OrgID)
	fmt.Fprintf(tw, "role\t%s\n", member.Member.Role)
	return tw.Flush()
}
