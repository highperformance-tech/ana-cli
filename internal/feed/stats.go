package feed

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

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
	fs := cli.NewFlagSet("feed stats")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, feedServicePath+"/GetFeedStats", struct{}{}, &raw); err != nil {
		return fmt.Errorf("feed stats: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed statsResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("feed stats: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "messagesToday\t%d\n", typed.MessagesToday)
	fmt.Fprintf(tw, "messagesAllTime\t%d\n", typed.MessagesAllTime)
	fmt.Fprintf(tw, "activeAgents\t%d\n", typed.ActiveAgents)
	fmt.Fprintf(tw, "dashboardsCreated\t%d\n", typed.DashboardsCreated)
	fmt.Fprintf(tw, "threadsCreated\t%d\n", typed.ThreadsCreated)
	fmt.Fprintf(tw, "playbooksCreated\t%d\n", typed.PlaybooksCreated)
	fmt.Fprintf(tw, "connectorsConfigured\t%d\n", typed.ConnectorsConfigured)
	fmt.Fprintf(tw, "connectorNames\t%s\n", joinOrDash(typed.ConnectorNames))
	fmt.Fprintf(tw, "activeAgentNames\t%s\n", joinOrDash(typed.ActiveAgentNames))
	return tw.Flush()
}

// joinOrDash renders a comma-joined list or "-" when empty so the column
// width stays sensible.
func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ", ")
}
