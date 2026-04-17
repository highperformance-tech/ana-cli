package chat

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/textql/ana-cli/internal/cli"
)

// listCmd implements `ana chat list` — GetChats with an empty body. The
// server also accepts paging/filter fields (memberOnly, sortBy, ...) but the
// v1 CLI prints whatever the default returns; flags for those can land later.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List chats (table by default, --json for raw JSON).\n" +
		"Usage: ana chat list"
}

// listResp is the narrow typed view used for table rendering. The catalog
// field for the title is `summary` (the brief calls it TITLE in the header);
// we keep the user-facing header as TITLE per the brief but read from summary.
type listResp struct {
	Chats []struct {
		ID        string `json:"id"`
		Summary   string `json:"summary"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"chats"`
}

func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("chat list")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChats", struct{}{}, &raw); err != nil {
		return fmt.Errorf("chat list: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed listResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("chat list: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tUPDATED")
	for _, ch := range typed.Chats {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.ID, ch.Summary, ch.UpdatedAt)
	}
	return tw.Flush()
}
