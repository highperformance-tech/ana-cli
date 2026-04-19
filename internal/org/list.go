package org

import (
	"context"
	"fmt"
	"sort"
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

// listOrganizationsResp narrows the response to the render columns. The catalog
// has many additional fields (theme, toolRestrictions, ...) which the decoder
// silently drops. DefaultConnectorID is *int64 so an absent field is
// distinguishable from a literal 0 — the catalog shows orgs omit the key
// entirely when no default exists.
type listOrganizationsResp struct {
	Organizations []struct {
		OrgID              string `json:"orgId"`
		OrganizationName   string `json:"organizationName"`
		DefaultConnectorID *int64 `json:"defaultConnectorId,omitempty"`
	} `json:"organizations"`
}

// Run issues ListOrganizations with an empty body and either prints a table
// sorted by name (case-insensitive for stable output across mixed-case names)
// or the raw payload under --json. The --json branch preserves server order
// since callers piping JSON may rely on it.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("org list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/ListOrganizations", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org list: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed listOrganizationsResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("org list: decode response: %w", err)
	}
	// Case-insensitive sort so "acme" and "Acme Inc" order intuitively instead
	// of splitting on ASCII case boundaries.
	sort.SliceStable(typed.Organizations, func(i, j int) bool {
		return strings.ToLower(typed.Organizations[i].OrganizationName) < strings.ToLower(typed.Organizations[j].OrganizationName)
	})
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "NAME\tORG ID\tDEFAULT CONNECTOR")
	for _, o := range typed.Organizations {
		var conn string
		if o.DefaultConnectorID != nil {
			conn = strconv.FormatInt(*o.DefaultConnectorID, 10)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", o.OrganizationName, o.OrgID, conn)
	}
	return tw.Flush()
}
