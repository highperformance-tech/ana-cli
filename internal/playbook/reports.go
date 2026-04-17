package playbook

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/textql/ana-cli/internal/cli"
)

// reportsCmd implements `ana playbook reports <id>` — GetPlaybookReports with
// `{playbookId: "..."}`. Table columns per the brief: RUN_ID, STATUS, RAN_AT.
//
// Catalog deviation: the captured report entries have no explicit `status` or
// `ranAt` fields. We map RUN_ID -> report `id`, STATUS -> `subject` (the
// closest human-readable label we have — defaults to "-" if missing), and
// RAN_AT -> `createdAt` (the only timestamp in the payload). --json still
// surfaces the raw response verbatim.
type reportsCmd struct{ deps Deps }

func (c *reportsCmd) Help() string {
	return "reports   List a playbook's report runs (RUN_ID/STATUS/RAN_AT table, --json for raw).\n" +
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
	fs := newFlagSet("playbook reports")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("playbook reports", fs.Args())
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybookReports", reportsReq{PlaybookID: id}, &raw); err != nil {
		return fmt.Errorf("playbook reports: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed reportsResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("playbook reports: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN_ID\tSTATUS\tRAN_AT")
	for _, r := range typed.Reports {
		status := r.Subject
		if status == "" {
			status = "-"
		}
		ranAt := r.CreatedAt
		if ranAt == "" {
			ranAt = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.ID, status, ranAt)
	}
	return tw.Flush()
}
