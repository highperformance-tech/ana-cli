package dashboard

import (
	"context"
	"fmt"
	"text/tabwriter"

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

// Run resolves the positional id, issues GetDashboard, then either dumps raw
// JSON (--json) or renders a compact key:value summary. The fallback (no
// `dashboard` key) prints raw JSON so we never silently swallow a response.
func (c *getCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("dashboard get")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requireID("dashboard get", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetDashboard", getReq{DashboardID: id}, &raw); err != nil {
		return fmt.Errorf("dashboard get: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	dash, ok := raw["dashboard"].(map[string]any)
	if !ok {
		return writeJSON(stdio.Stdout, raw)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	for _, k := range []string{"id", "name", "orgId", "creatorId"} {
		if v, ok := dash[k]; ok {
			fmt.Fprintf(tw, "%s:\t%v\n", k, v)
		}
	}
	if code, ok := dash["code"].(string); ok {
		fmt.Fprintf(tw, "code:\t%d bytes\n", len(code))
	}
	return tw.Flush()
}
