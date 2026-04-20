package dashboard

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newFoldersGroup returns the nested `dashboard folders` verb group. Only
// `list` is exposed today; create/update/delete have not been captured in
// the API catalog.
func newFoldersGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "List dashboard folders.",
		Children: map[string]cli.Command{
			"list": &foldersListCmd{deps: deps},
		},
	}
}

// foldersListCmd implements `ana dashboard folders list` — ListDashboardFolders
// with an empty body.
type foldersListCmd struct{ deps Deps }

func (c *foldersListCmd) Help() string {
	return "list   List dashboard folders (table by default, --json for raw).\n" +
		"Usage: ana dashboard folders list"
}

// folderEntry is the per-folder projection used to render the ID/NAME table.
type folderEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// foldersResp reflects the catalogued (empty) response shape plus the
// likely-camelCase field names the webapp sorts by. The captured sample is
// `{}` so any field we name here is a best-effort guess; the key detail is
// that unknown fields decode silently and `--json` still emits raw.
type foldersResp struct {
	Folders []folderEntry `json:"folders"`
}

// Run issues ListDashboardFolders, then either dumps raw JSON or renders an
// ID/NAME table sorted by name for determinism.
func (c *foldersListCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("dashboard folders list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/ListDashboardFolders", struct{}{}, &raw); err != nil {
		return fmt.Errorf("dashboard folders list: %w", err)
	}
	var typed foldersResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *foldersResp) error {
		slices.SortFunc(t.Folders, func(a, b folderEntry) int {
			return cmp.Compare(a.Name, b.Name)
		})
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME")
		for _, f := range t.Folders {
			fmt.Fprintf(tw, "%s\t%s\n", f.ID, f.Name)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("dashboard folders list: %w", err)
	}
	return nil
}
