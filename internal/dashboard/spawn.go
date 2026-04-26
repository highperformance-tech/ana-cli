package dashboard

import (
	"context"
	"fmt"
	"io"

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

// spawnResp captures the only field we print by default; absence triggers
// raw JSON fallback from the render closure.
type spawnResp struct {
	RefreshedAt string `json:"refreshedAt"`
}

// Run resolves the id, POSTs SpawnDashboard, and prints either the raw
// response (--json) or the refreshedAt field. If refreshedAt is absent we
// fall back to raw JSON so we never lose information.
func (c *spawnCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	id, err := cli.RequireStringID("dashboard spawn", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/SpawnDashboard", spawnReq{DashboardID: id}, &raw); err != nil {
		return fmt.Errorf("dashboard spawn: %w", err)
	}
	var typed spawnResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *spawnResp) error {
		if t.RefreshedAt == "" {
			return cli.WriteJSON(w, raw)
		}
		_, err := fmt.Fprintf(w, "spawned %s (refreshedAt=%s)\n", id, t.RefreshedAt)
		return err
	}); err != nil {
		return fmt.Errorf("dashboard spawn: %w", err)
	}
	return nil
}
