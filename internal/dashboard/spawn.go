package dashboard

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// spawnCmd implements `ana dashboard spawn <id>` — SpawnDashboard. The
// captured response has only `{"refreshedAt":"..."}` so the default render
// prints that timestamp; `--json` dumps the raw response. There is no
// dedicated "spawn URL" in the response shape observed — the webapp uses
// CheckDashboardHealth afterwards to discover streamlit/embed URLs, and
// `ana dashboard health` exposes that.
type spawnCmd struct{ deps Deps }

func (c *spawnCmd) Help() string {
	return "spawn <id>   Spawn a dashboard runtime and print refreshedAt.\n" +
		"Usage: ana dashboard spawn <id>"
}

// spawnReq mirrors the catalog's `{"dashboardId":"..."}` request body.
type spawnReq struct {
	DashboardID string `json:"dashboardId"`
}

// Run resolves the id, POSTs SpawnDashboard, and prints either the raw
// response (--json) or the refreshedAt field. If refreshedAt is absent we
// fall back to raw JSON so we never lose information.
func (c *spawnCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("dashboard spawn")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("dashboard spawn", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/SpawnDashboard", spawnReq{DashboardID: id}, &raw); err != nil {
		return fmt.Errorf("dashboard spawn: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	if ts, ok := raw["refreshedAt"].(string); ok {
		fmt.Fprintf(stdio.Stdout, "spawned %s (refreshedAt=%s)\n", id, ts)
		return nil
	}
	return cli.WriteJSON(stdio.Stdout, raw)
}
