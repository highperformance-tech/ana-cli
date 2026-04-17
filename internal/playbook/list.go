package playbook

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd implements `ana playbook list` — GetPlaybooks with `{}`. Table
// columns: ID, NAME, SCHEDULE (cronString if present, else "-").
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List playbooks (ID/NAME/SCHEDULE table, --json for raw).\n" +
		"Usage: ana playbook list"
}

// listResp narrows the fields we render. The catalog has many more (orgId,
// memberId, prompt, owner, paradigmOptions, ...); the decoder drops them.
type listResp struct {
	Playbooks []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		CronString string `json:"cronString"`
	} `json:"playbooks"`
}

// Run issues GetPlaybooks and prints either a table or the raw payload.
// Empty CronString cells render as "-" so tabwriter keeps the column aligned
// for playbooks without a schedule.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("playbook list")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybooks", struct{}{}, &raw); err != nil {
		return fmt.Errorf("playbook list: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed listResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("playbook list: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSCHEDULE")
	for _, p := range typed.Playbooks {
		sched := p.CronString
		if sched == "" {
			sched = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Name, sched)
	}
	return tw.Flush()
}
