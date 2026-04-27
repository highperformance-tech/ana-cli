package dashboard

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// healthCmd implements `ana dashboard health <id>` — CheckDashboardHealth.
//
// Catalog deviation: the brief suggests request body `{"dashboardId":"uuid"}`
// but the captured sample uses `{"dashboardIds":["uuid"]}` (plural, array)
// and returns `{"dashboards":[{...}]}` with one entry per requested id. We
// follow the catalog and flag this in the PR/report.
type healthCmd struct{ deps Deps }

func (c *healthCmd) Help() string {
	return "health <id>   Show a dashboard's runtime health (HEALTHY / UNHEALTHY + detail).\n" +
		"Usage: ana dashboard health <id>"
}

// healthReq mirrors the catalog's plural `{"dashboardIds":["..."]}` body.
type healthReq struct {
	DashboardIDs []string `json:"dashboardIds"`
}

// healthResp is the narrow projection of the catalog's response. We keep the
// fields the webapp surfaces in its health indicator: status, message, and
// the streamlit/embed URLs so ops can curl or open the running dashboard.
type healthResp struct {
	Dashboards []struct {
		DashboardID  string `json:"dashboardId"`
		Status       string `json:"status"`
		Message      string `json:"message"`
		StreamlitURL string `json:"streamlitUrl"`
		EmbedURL     string `json:"embedUrl"`
		RefreshedAt  string `json:"refreshedAt"`
	} `json:"dashboards"`
}

// Run resolves the id, POSTs CheckDashboardHealth wrapped in a single-element
// array, then prints either raw JSON (--json) or a compact
// "<id> HEALTHY" / "<id> UNHEALTHY: <message>" summary. If the response
// contains no matching entry we surface an error rather than silently
// succeeding.
func (c *healthCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("dashboard health", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("dashboard health", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/CheckDashboardHealth", healthReq{DashboardIDs: []string{id}}, &raw); err != nil {
		return fmt.Errorf("dashboard health: %w", err)
	}
	var typed healthResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *healthResp) error {
		if len(t.Dashboards) == 0 {
			return fmt.Errorf("no dashboard entry for %s: %w", id, cli.ErrUsage)
		}
		d := t.Dashboards[0]
		label := healthLabel(d.Status)
		if d.Message != "" {
			fmt.Fprintf(w, "%s %s: %s\n", d.DashboardID, label, d.Message)
		} else {
			fmt.Fprintf(w, "%s %s\n", d.DashboardID, label)
		}
		if d.StreamlitURL != "" {
			fmt.Fprintf(w, "streamlitUrl: %s\n", d.StreamlitURL)
		}
		if d.EmbedURL != "" {
			fmt.Fprintf(w, "embedUrl: %s\n", d.EmbedURL)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("dashboard health: %w", err)
	}
	return nil
}

// healthLabel collapses the HEALTH_STATUS_* enum to a compact label. Anything
// we don't recognise is passed through unchanged so the user still sees the
// raw protobuf value.
func healthLabel(s string) string {
	switch s {
	case "HEALTH_STATUS_HEALTHY":
		return "HEALTHY"
	case "HEALTH_STATUS_UNHEALTHY":
		return "UNHEALTHY"
	case "HEALTH_STATUS_UNSPECIFIED", "":
		return "UNKNOWN"
	}
	return strings.TrimPrefix(s, "HEALTH_STATUS_")
}
