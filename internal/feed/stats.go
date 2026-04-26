package feed

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// statsCmd implements `ana feed stats` — GetFeedStats with `{}`. Key-value
// view of the headline counters plus joined connector/agent name lists.
type statsCmd struct{ deps Deps }

func (c *statsCmd) Help() string {
	return "stats   Show feed-wide counters (key/value view, --json for raw).\n" +
		"Usage: ana feed stats"
}

// statsResp is the compact typed projection of the GetFeedStats response.
type statsResp struct {
	MessagesToday        int64    `json:"messagesToday"`
	MessagesAllTime      int64    `json:"messagesAllTime"`
	ActiveAgents         int64    `json:"activeAgents"`
	DashboardsCreated    int64    `json:"dashboardsCreated"`
	ThreadsCreated       int64    `json:"threadsCreated"`
	PlaybooksCreated     int64    `json:"playbooksCreated"`
	ConnectorsConfigured int64    `json:"connectorsConfigured"`
	ConnectorNames       []string `json:"connectorNames"`
	ActiveAgentNames     []string `json:"activeAgentNames"`
}

func (c *statsCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	var raw map[string]any
	if err := c.deps.Unary(ctx, feedServicePath+"/GetFeedStats", struct{}{}, &raw); err != nil {
		return fmt.Errorf("feed stats: %w", err)
	}
	var typed statsResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *statsResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintf(tw, "messagesToday\t%d\n", t.MessagesToday)
		fmt.Fprintf(tw, "messagesAllTime\t%d\n", t.MessagesAllTime)
		fmt.Fprintf(tw, "activeAgents\t%d\n", t.ActiveAgents)
		fmt.Fprintf(tw, "dashboardsCreated\t%d\n", t.DashboardsCreated)
		fmt.Fprintf(tw, "threadsCreated\t%d\n", t.ThreadsCreated)
		fmt.Fprintf(tw, "playbooksCreated\t%d\n", t.PlaybooksCreated)
		fmt.Fprintf(tw, "connectorsConfigured\t%d\n", t.ConnectorsConfigured)
		fmt.Fprintf(tw, "connectorNames\t%s\n", joinOrDash(t.ConnectorNames))
		fmt.Fprintf(tw, "activeAgentNames\t%s\n", joinOrDash(t.ActiveAgentNames))
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("feed stats: %w", err)
	}
	return nil
}

// joinOrDash renders a comma-joined list or "-" when empty so the column
// width stays sensible.
func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ", ")
}
