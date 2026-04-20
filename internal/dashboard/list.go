package dashboard

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd implements `ana dashboard list`. It calls ListDashboards with an
// empty body and renders ID/NAME/FOLDER. The FOLDER column is whichever of
// the folder-shaped fields the API surfaces first (the response schema has
// not been fully captured — the sample in api-catalog has no folder data);
// we fall back gracefully rather than failing the render.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List dashboards (table by default, --json for raw).\n" +
		"Usage: ana dashboard list"
}

// listResp is the narrow shape we render. Fields we don't care about
// (`code`, `orgId`, `creatorId`, etc.) are silently dropped by the decoder.
type listResp struct {
	Dashboards []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		FolderID   string `json:"folderId"`
		FolderName string `json:"folderName"`
	} `json:"dashboards"`
}

// Run issues ListDashboards then either dumps raw JSON or prints an
// ID/NAME/FOLDER table. FOLDER prefers a human-readable folderName, falls
// back to folderId, and renders an em-dash when neither is set.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("dashboard list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/ListDashboards", struct{}{}, &raw); err != nil {
		return fmt.Errorf("dashboard list: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed listResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("dashboard list: decode response: %w", err)
	}
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "ID\tNAME\tFOLDER")
	for _, d := range typed.Dashboards {
		folder := cli.DashIfEmpty(cli.FirstNonEmpty(d.FolderName, d.FolderID))
		fmt.Fprintf(tw, "%s\t%s\t%s\n", d.ID, d.Name, folder)
	}
	return tw.Flush()
}
