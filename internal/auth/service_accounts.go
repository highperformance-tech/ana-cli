package auth

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newServiceAccountsGroup wires `auth service-accounts` (list/create/delete).
func newServiceAccountsGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage service accounts.",
		Children: map[string]cli.Command{
			"list":   &saListCmd{deps: deps},
			"create": &saCreateCmd{deps: deps},
			"delete": &saDeleteCmd{deps: deps},
		},
	}
}

// ---- list ----

type saListCmd struct{ deps Deps }

func (c *saListCmd) Help() string {
	return "service-accounts list   List service accounts (table, --json for raw).\n" +
		"Usage: ana auth service-accounts list"
}

type listServiceAccountsResp struct {
	ServiceAccounts []struct {
		MemberID    string `json:"memberId"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	} `json:"serviceAccounts"`
}

func (c *saListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("auth service-accounts list: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListServiceAccounts", struct{}{}, &raw); err != nil {
		return fmt.Errorf("auth service-accounts list: %w", translateErr(err))
	}
	var typed listServiceAccountsResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listServiceAccountsResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION")
		for _, sa := range t.ServiceAccounts {
			desc := sa.Description
			if desc == "" {
				desc = sa.Email
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", sa.MemberID, sa.DisplayName, desc)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("auth service-accounts list: %w", err)
	}
	return nil
}

// ---- create ----

type saCreateCmd struct {
	deps Deps
	name string
	desc string
}

func (c *saCreateCmd) Help() string {
	return "service-accounts create   Create a service account.\n" +
		"Usage: ana auth service-accounts create --name <name> [--description <text>]"
}

func (c *saCreateCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.name, "name", "", "human-readable name (required)")
	fs.StringVar(&c.desc, "description", "", "optional description")
}

type createServiceAccountReq struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type createServiceAccountResp struct {
	MemberID string `json:"memberId"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

func (c *saCreateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("auth service-accounts create: unexpected positional arguments: %v", args)
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "auth service-accounts create", "name"); err != nil {
		return err
	}
	if c.name == "" {
		return cli.UsageErrf("auth service-accounts create: --name must not be empty")
	}
	req := createServiceAccountReq{Name: c.name, Description: c.desc}
	var resp createServiceAccountResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/CreateServiceAccount", req, &resp); err != nil {
		return fmt.Errorf("auth service-accounts create: %w", translateErr(err))
	}
	echoed := resp.Name
	if echoed == "" {
		echoed = c.name
	}
	fmt.Fprintf(stdio.Stdout, "%s %s\n", resp.MemberID, echoed)
	return nil
}

// ---- delete ----

type saDeleteCmd struct{ deps Deps }

func (c *saDeleteCmd) Help() string {
	return "service-accounts delete   Delete a service account by id.\n" +
		"Usage: ana auth service-accounts delete <id>\n" +
		"\n" +
		"<id> is the memberId shown by `ana auth service-accounts list`. memberId is\n" +
		"per-org, so the same id will not resolve against a different profile. Deleting\n" +
		"cascades to revoke all API keys issued by the account."
}

type deleteServiceAccountReq struct {
	MemberID string `json:"memberId"`
}

func (c *saDeleteCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) > 1 {
		return cli.UsageErrf("auth service-accounts delete: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("auth service-accounts delete", args)
	if err != nil {
		return err
	}
	req := deleteServiceAccountReq{MemberID: id}
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/DeleteServiceAccount", req, nil); err != nil {
		return fmt.Errorf("auth service-accounts delete: %w", translateErr(err))
	}
	fmt.Fprintf(stdio.Stdout, "deleted %s\n", id)
	return nil
}
