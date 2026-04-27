package org

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd prints every organization the authenticated member belongs to.
// Path: /rpc/public/textql.rpc.public.auth.PublicAuthService/ListOrganizations.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List organizations the authenticated member belongs to.\n" +
		"Usage: ana org list"
}

// organizationEntry is the per-org projection used to render the list table.
type organizationEntry struct {
	OrgID              string `json:"orgId"`
	OrganizationName   string `json:"organizationName"`
	DefaultConnectorID *int64 `json:"defaultConnectorId,omitempty"`
}

// listOrganizationsResp narrows the response to the render columns. The catalog
// has many additional fields (theme, toolRestrictions, ...) which the decoder
// silently drops. DefaultConnectorID is *int64 so an absent field is
// distinguishable from a literal 0 — the catalog shows orgs omit the key
// entirely when no default exists.
type listOrganizationsResp struct {
	Organizations []organizationEntry `json:"organizations"`
}

// Run issues ListOrganizations with an empty body and either prints a table
// sorted by name (case-insensitive for stable output across mixed-case names)
// or the raw payload under --json. The --json branch preserves server order
// since callers piping JSON may rely on it.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireNoPositionals("org list", args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/ListOrganizations", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org list: %w", err)
	}
	var typed listOrganizationsResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listOrganizationsResp) error {
		slices.SortStableFunc(t.Organizations, func(a, b organizationEntry) int {
			return cmp.Compare(strings.ToLower(a.OrganizationName), strings.ToLower(b.OrganizationName))
		})
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "NAME\tORG ID\tDEFAULT CONNECTOR")
		for _, o := range t.Organizations {
			var conn string
			if o.DefaultConnectorID != nil {
				conn = strconv.FormatInt(*o.DefaultConnectorID, 10)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", o.OrganizationName, o.OrgID, conn)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("org list: %w", err)
	}
	return nil
}
