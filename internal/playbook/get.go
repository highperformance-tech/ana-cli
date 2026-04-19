package playbook

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// getCmd implements `ana playbook get <id>` — GetPlaybook with
// `{playbookId: "..."}`. Default output is a two-column list of the main
// fields; --json prints the full raw response.
type getCmd struct{ deps Deps }

func (c *getCmd) Help() string {
	return "get   Show a playbook's main fields (--json for raw).\n" +
		"Usage: ana playbook get <id>"
}

// getReq is the exact wire shape — catalog confirms a single `playbookId`
// field (plus an optional `limit` for reports; we omit it here and let the
// server use its default).
type getReq struct {
	PlaybookID string `json:"playbookId"`
}

// getResp is the compact typed projection. We pull only the fields the
// pretty-print path renders; everything else remains available via --json.
type getResp struct {
	Playbook struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Status            string `json:"status"`
		TriggerType       string `json:"triggerType"`
		CronString        string `json:"cronString"`
		CreatedAt         string `json:"createdAt"`
		UpdatedAt         string `json:"updatedAt"`
		ParadigmType      string `json:"paradigmType"`
		ReportOutputStyle string `json:"reportOutputStyle"`
		LatestChatID      string `json:"latestChatId"`
		Owner             struct {
			MemberEmail string `json:"memberEmail"`
		} `json:"owner"`
	} `json:"playbook"`
}

func (c *getCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("playbook get")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("playbook get", fs.Args())
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybook", getReq{PlaybookID: id}, &raw); err != nil {
		return fmt.Errorf("playbook get: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed getResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("playbook get: decode response: %w", err)
	}
	// A missing `playbook` envelope falls through to --json so the user sees
	// the response shape rather than a block of empty fields.
	if typed.Playbook.ID == "" {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	p := typed.Playbook
	tw := cli.NewTableWriter(stdio.Stdout)
	// Two-column key/value list. Keys mirror the wire-level camelCase so users
	// searching docs land on the same identifier.
	fmt.Fprintf(tw, "id\t%s\n", p.ID)
	fmt.Fprintf(tw, "name\t%s\n", p.Name)
	fmt.Fprintf(tw, "status\t%s\n", p.Status)
	fmt.Fprintf(tw, "triggerType\t%s\n", p.TriggerType)
	fmt.Fprintf(tw, "cronString\t%s\n", p.CronString)
	fmt.Fprintf(tw, "paradigmType\t%s\n", p.ParadigmType)
	fmt.Fprintf(tw, "reportOutputStyle\t%s\n", p.ReportOutputStyle)
	fmt.Fprintf(tw, "owner\t%s\n", p.Owner.MemberEmail)
	fmt.Fprintf(tw, "latestChatId\t%s\n", p.LatestChatID)
	fmt.Fprintf(tw, "createdAt\t%s\n", p.CreatedAt)
	fmt.Fprintf(tw, "updatedAt\t%s\n", p.UpdatedAt)
	return tw.Flush()
}
