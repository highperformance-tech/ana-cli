package org

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newRolesGroup wires `org roles list`. Like members, the wrapping group is
// a forward-compatibility hedge so future verbs (create, delete, ...) land
// under the same prefix without restructuring the tree.
func newRolesGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect organization roles.",
		Children: map[string]cli.Command{
			"list": &rolesListCmd{deps: deps},
		},
	}
}

type rolesListCmd struct{ deps Deps }

func (c *rolesListCmd) Help() string {
	return "roles list   List RBAC roles (ID/NAME table, --json for raw).\n" +
		"Usage: ana org roles list"
}

// listRolesResp narrows the fields we render; the capture also exposes
// description, isSystem, createdAt, allowModelChoice. Dropping them now keeps
// table rendering predictable without obscuring their availability under
// --json (which dumps the full payload).
type listRolesResp struct {
	Roles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"roles"`
}

// Run issues ListRoles with an empty body and renders a two-column table or
// the raw JSON payload under --json.
func (c *rolesListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireNoPositionals("org roles list", args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListRoles", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org roles list: %w", err)
	}
	var typed listRolesResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listRolesResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME")
		for _, r := range t.Roles {
			fmt.Fprintf(tw, "%s\t%s\n", r.ID, r.Name)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("org roles list: %w", err)
	}
	return nil
}
