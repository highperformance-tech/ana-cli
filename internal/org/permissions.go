package org

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newPermissionsGroup wires `org permissions list`. Permissions are server-
// defined and readonly at this layer, so the group currently has only `list`.
func newPermissionsGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect RBAC permissions.",
		Children: map[string]cli.Command{
			"list": &permissionsListCmd{deps: deps},
		},
	}
}

type permissionsListCmd struct{ deps Deps }

func (c *permissionsListCmd) Help() string {
	return "permissions list   List RBAC permissions (ID/NAME table, --json for raw).\n" +
		"Usage: ana org permissions list"
}

// listPermissionsResp narrows the fields we render. The catalog shows no
// `name` field — permissions are identified by (resource, action) — so we
// synthesize NAME as `resource:action`, which matches how they're referenced
// in access-control logs (e.g. `api_key.created`, but colon-joined for RBAC).
type listPermissionsResp struct {
	Permissions []struct {
		ID       string `json:"id"`
		Resource string `json:"resource"`
		Action   string `json:"action"`
	} `json:"permissions"`
}

// Run issues ListPermissions with an empty body and renders an ID/NAME table
// or the raw JSON payload under --json. A row missing both resource and
// action falls back to "-" so the column stays aligned.
func (c *permissionsListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListPermissions", struct{}{}, &raw); err != nil {
		return fmt.Errorf("org permissions list: %w", err)
	}
	var typed listPermissionsResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listPermissionsResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME")
		for _, p := range t.Permissions {
			name := permissionName(p.Resource, p.Action)
			fmt.Fprintf(tw, "%s\t%s\n", p.ID, name)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("org permissions list: %w", err)
	}
	return nil
}

// permissionName joins resource and action into a single NAME column value.
// Either component alone is acceptable; if both are empty we emit "-" so the
// column keeps its width.
func permissionName(resource, action string) string {
	switch {
	case resource != "" && action != "":
		return resource + ":" + action
	case resource != "":
		return resource
	case action != "":
		return action
	default:
		return "-"
	}
}
