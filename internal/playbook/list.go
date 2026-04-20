package playbook

import (
	"context"
	"fmt"
	"io"

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
	fs := cli.NewFlagSet("playbook list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybooks", struct{}{}, &raw); err != nil {
		return fmt.Errorf("playbook list: %w", err)
	}
	var typed listResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME\tSCHEDULE")
		for _, p := range t.Playbooks {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Name, cli.DashIfEmpty(p.CronString))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("playbook list: %w", err)
	}
	return nil
}
