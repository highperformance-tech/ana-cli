package org

import (
	"context"
	"fmt"
	"text/tabwriter"

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
	fs := newFlagSet("org show")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org show: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed getOrganizationResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("org show: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	// Two-column key/value list. Keys mirror the wire-level camelCase so users
	// searching docs land on the same identifier.
	fmt.Fprintf(tw, "organizationName\t%s\n", typed.Organization.OrganizationName)
	fmt.Fprintf(tw, "orgId\t%s\n", typed.Organization.OrgID)
	fmt.Fprintf(tw, "createdAt\t%s\n", typed.Organization.CreatedAt)
	return tw.Flush()
}
