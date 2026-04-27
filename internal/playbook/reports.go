package playbook

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// reportsCmd implements `ana playbook reports <id>` — GetPlaybookReports with
// `{playbookId: "..."}`. Table columns: RUN_ID, SUBJECT, RAN_AT.
//
// Catalog deviation: the captured report entries have no explicit `status` or
// `ranAt` fields. We map RUN_ID -> report `id`, SUBJECT -> `subject` (the
// closest human-readable label we have — defaults to "-" if missing), and
// RAN_AT -> `createdAt` (the only timestamp in the payload). --json still
// surfaces the raw response verbatim.
type reportsCmd struct{ deps Deps }

func (c *reportsCmd) Help() string {
	return "reports   List a playbook's report runs (RUN_ID/SUBJECT/RAN_AT table, --json for raw).\n" +
		"Usage: ana playbook reports <id>"
}

type reportsReq struct {
	PlaybookID string `json:"playbookId"`
}

type reportsResp struct {
	Reports []struct {
		ID        string `json:"id"`
		Subject   string `json:"subject"`
		CreatedAt string `json:"createdAt"`
	} `json:"reports"`
}

func (c *reportsCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("playbook reports", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("playbook reports", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybookReports", reportsReq{PlaybookID: id}, &raw); err != nil {
		return fmt.Errorf("playbook reports: %w", err)
	}
	var typed reportsResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *reportsResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "RUN_ID\tSUBJECT\tRAN_AT")
		for _, r := range t.Reports {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.ID, cli.DashIfEmpty(r.Subject), cli.DashIfEmpty(r.CreatedAt))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("playbook reports: %w", err)
	}
	return nil
}
