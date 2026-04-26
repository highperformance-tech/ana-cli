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
type whoamiCmd struct{ deps Deps }

func (c *whoamiCmd) Help() string {
	return "whoami   Print email, org, and role (tabwriter) or both raw responses with --json.\n" +
		"Usage: ana auth whoami"
}

type getMemberResp struct {
	Member struct {
		MemberID     string `json:"memberId"`
		EmailAddress string `json:"emailAddress"`
		Name         string `json:"name"`
		OrgID        string `json:"orgId"`
		Role         string `json:"role"`
	} `json:"member"`
}

type getOrganizationResp struct {
	Organization struct {
		OrgID            string `json:"orgId"`
		OrganizationName string `json:"organizationName"`
	} `json:"organization"`
}

func (c *whoamiCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("auth whoami: unexpected positional arguments: %v", args)
	}
	cfg, err := c.deps.LoadCfg()
	if err != nil {
		return fmt.Errorf("auth whoami: load config: %w", err)
	}
	if cfg.Token == "" {
		return fmt.Errorf("auth whoami: %w", ErrNotLoggedIn)
	}

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
	if err := errors.Join(memberErr, orgErr); err != nil {
		return fmt.Errorf("auth whoami: %w", err)
	}

	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, map[string]any{
			"member":       memberRaw,
			"organization": orgRaw,
		})
	}

	var member getMemberResp
	if err := cli.Remarshal(memberRaw, &member); err != nil {
		return fmt.Errorf("auth whoami: decode response: %w", err)
	}
	var org getOrganizationResp
	if err := cli.Remarshal(orgRaw, &org); err != nil {
		return fmt.Errorf("auth whoami: decode response: %w", err)
	}
	if member.Member.EmailAddress == "" {
		return fmt.Errorf("auth whoami: response missing member.emailAddress")
	}

	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintf(tw, "email\t%s\n", member.Member.EmailAddress)
	fmt.Fprintf(tw, "organization\t%s\n", org.Organization.OrganizationName)
	fmt.Fprintf(tw, "orgId\t%s\n", member.Member.OrgID)
	fmt.Fprintf(tw, "role\t%s\n", member.Member.Role)
	return tw.Flush()
}
