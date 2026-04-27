package org

import (
	"context"
	"fmt"
	"io"

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
	if err := cli.RequireNoPositionals("org members list", args); err != nil {
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
	var typed listOrganizationMembersResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listOrganizationMembersResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tEMAIL\tROLE")
		for _, m := range t.Members {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", m.MemberID, m.EmailAddress, cli.DashIfEmpty(m.Role))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("org members list: %w", err)
	}
	return nil
}
