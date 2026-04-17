package org

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newMembersGroup wires `org members` — currently a single `list` child, but
// kept as a group so future verbs (invite, remove, ...) slot in without a
// breaking reshuffle of the command tree.
func newMembersGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage organization members.",
		Children: map[string]cli.Command{
			"list": &membersListCmd{deps: deps},
		},
	}
}

type membersListCmd struct{ deps Deps }

func (c *membersListCmd) Help() string {
	return "members list   List organization members (ID/EMAIL/ROLE table, --json for raw).\n" +
		"Usage: ana org members list"
}

// listOrganizationMembersResp narrows the fields we render. The catalog has
// many more (profileImageUrl, paradigmParams, ...). We use `memberId` as the
// ID column (UUID) rather than the numeric `id`, matching how other RPCs in
// this service refer to members.
type listOrganizationMembersResp struct {
	Members []struct {
		MemberID     string `json:"memberId"`
		EmailAddress string `json:"emailAddress"`
		Role         string `json:"role"`
	} `json:"members"`
}

// Run issues ListOrganizationMembers (server requires orgId in the request
// body — it won't infer from the token) and either prints a table or the
// raw payload under --json. An empty Role cell renders as "-" so tabwriter
// keeps the column aligned for old accounts without a role.
func (c *membersListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("org members list")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	var orgResp struct {
		Organization struct {
			OrgID string `json:"orgId"`
		} `json:"organization"`
	}
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization", struct{}{}, &orgResp); err != nil {
		return fmt.Errorf("org members list: resolve orgId: %w", err)
	}
	var raw map[string]any
	req := map[string]any{"orgId": orgResp.Organization.OrgID}
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.settings.SettingsService/ListOrganizationMembers", req, &raw); err != nil {
		return fmt.Errorf("org members list: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed listOrganizationMembersResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("org members list: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tEMAIL\tROLE")
	for _, m := range typed.Members {
		role := m.Role
		if role == "" {
			role = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", m.MemberID, m.EmailAddress, role)
	}
	return tw.Flush()
}
