package auth

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newServiceAccountsGroup wires `auth service-accounts` (list/create/delete).
// The group summary is short because root-level help already disambiguates.
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

// listServiceAccountsResp narrows the fields we render. The catalog exposes
// memberId (not a separate serviceAccountId) as the primary id, along with a
// displayName and email; we show those as ID/NAME/DESCRIPTION respectively.
type listServiceAccountsResp struct {
	ServiceAccounts []struct {
		MemberID    string `json:"memberId"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Description string `json:"description"`
	} `json:"serviceAccounts"`
}

// Run issues ListServiceAccounts and prints a table (or JSON under --json).
// Description in the response isn't always populated; we fall back to the
// auto-generated email so the third column isn't blank in practice.
func (c *saListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth service-accounts list")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/ListServiceAccounts", struct{}{}, &raw); err != nil {
		return fmt.Errorf("auth service-accounts list: %w", translateErr(err))
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed listServiceAccountsResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("auth service-accounts list: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION")
	for _, sa := range typed.ServiceAccounts {
		desc := sa.Description
		if desc == "" {
			desc = sa.Email
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", sa.MemberID, sa.DisplayName, desc)
	}
	return tw.Flush()
}

// ---- create ----

type saCreateCmd struct{ deps Deps }

func (c *saCreateCmd) Help() string {
	return "service-accounts create   Create a service account.\n" +
		"Usage: ana auth service-accounts create --name <name> [--description <text>]"
}

// createServiceAccountReq. Description is send-only (omitempty) because the
// captured sample doesn't include it; the server tolerates extra camelCase
// fields as long as they aren't snake_case duplicates.
type createServiceAccountReq struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// createServiceAccountResp mirrors the captured sample: memberId + email.
type createServiceAccountResp struct {
	MemberID string `json:"memberId"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

// Run enforces --name, issues the RPC, and prints `<memberId> <name>` as the
// minimal human-readable confirmation. The task spec says "print
// {serviceAccountId, name}" — the server returns memberId for that role.
func (c *saCreateCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth service-accounts create")
	name := fs.String("name", "", "human-readable name (required)")
	desc := fs.String("description", "", "optional description")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *name == "" {
		return usageErrf("auth service-accounts create: --name is required")
	}
	req := createServiceAccountReq{Name: *name, Description: *desc}
	var resp createServiceAccountResp
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/CreateServiceAccount", req, &resp); err != nil {
		return fmt.Errorf("auth service-accounts create: %w", translateErr(err))
	}
	// Name is echoed from the request if the response omits it, so users see
	// something meaningful even if the server keeps its reply minimal.
	echoed := resp.Name
	if echoed == "" {
		echoed = *name
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

// deleteServiceAccountReq uses `memberId` per the catalog's DeleteServiceAccount
// sample — note the task spec's `serviceAccountId` name is inaccurate.
type deleteServiceAccountReq struct {
	MemberID string `json:"memberId"`
}

func (c *saDeleteCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("auth service-accounts delete")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrf("auth service-accounts delete: <id> positional argument required")
	}
	id := fs.Arg(0)
	req := deleteServiceAccountReq{MemberID: id}
	if err := c.deps.Unary(ctx, "/rpc/public/textql.rpc.public.rbac.RBACService/DeleteServiceAccount", req, nil); err != nil {
		return fmt.Errorf("auth service-accounts delete: %w", translateErr(err))
	}
	fmt.Fprintf(stdio.Stdout, "deleted %s\n", id)
	return nil
}
