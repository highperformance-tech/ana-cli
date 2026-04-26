package org

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// showCmd prints the org's name, id, and creation timestamp (or full JSON).
// Path: /rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization.
type showCmd struct{ deps Deps }

func (c *showCmd) Help() string {
	return "show   Print the current organization's name, id, and createdAt (--json for raw).\n" +
		"Usage: ana org show"
}

// getOrganizationResp narrows the fields we render. The capture has many more
// (theme, toolRestrictions, ...); the decoder silently drops them.
type getOrganizationResp struct {
	Organization struct {
		OrgID            string `json:"orgId"`
		OrganizationName string `json:"organizationName"`
		CreatedAt        string `json:"createdAt"`
	} `json:"organization"`
}

// Run issues GetOrganization and prints either a two-column list or the raw
// JSON payload. The two-column layout uses tabwriter with a small gutter so
// field: value pairs stay visually aligned even if names grow.
func (c *showCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("org show: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org show: %w", err)
	}
	var typed getOrganizationResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *getOrganizationResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintf(tw, "organizationName\t%s\n", t.Organization.OrganizationName)
		fmt.Fprintf(tw, "orgId\t%s\n", t.Organization.OrgID)
		fmt.Fprintf(tw, "createdAt\t%s\n", t.Organization.CreatedAt)
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("org show: %w", err)
	}
	return nil
}
