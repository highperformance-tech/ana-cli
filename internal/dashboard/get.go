package dashboard

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// getCmd implements `ana dashboard get <id>` — GetDashboard with a single
// `dashboardId` request field. Default rendering is a short summary of key
// fields (id, name, orgId, creatorId, code length); `--json` dumps the
// full raw response including the (potentially very long) Streamlit `code`
// block.
type getCmd struct{ deps Deps }

func (c *getCmd) Help() string {
	return "get <id>   Show a dashboard by id (summary by default, --json for raw).\n" +
		"Usage: ana dashboard get <id>"
}

// getReq mirrors the catalog's `{"dashboardId":"..."}` request body.
type getReq struct {
	DashboardID string `json:"dashboardId"`
}

// getResp keeps the dashboard envelope as an untyped map so the renderer
// can probe the handful of keys it cares about without forcing every field.
type getResp struct {
	Dashboard map[string]any `json:"dashboard"`
}

// Run resolves the positional id, issues GetDashboard, then either dumps raw
// JSON (--json) or renders a compact key:value summary. The fallback (no
// `dashboard` key) prints raw JSON so we never silently swallow a response.
func (c *getCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	id, err := cli.RequireStringID("dashboard get", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetDashboard", getReq{DashboardID: id}, &raw); err != nil {
		return fmt.Errorf("dashboard get: %w", err)
	}
	var typed getResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *getResp) error {
		if t.Dashboard == nil {
			return cli.WriteJSON(w, raw)
		}
		tw := cli.NewTableWriter(w)
		for _, k := range []string{"id", "name", "orgId", "creatorId"} {
			if v, ok := t.Dashboard[k]; ok {
				fmt.Fprintf(tw, "%s:\t%v\n", k, v)
			}
		}
		if code, ok := t.Dashboard["code"].(string); ok {
			fmt.Fprintf(tw, "code:\t%d bytes\n", len(code))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("dashboard get: %w", err)
	}
	return nil
}
