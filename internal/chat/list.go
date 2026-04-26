package chat

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
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
	if len(args) != 0 {
		return cli.UsageErrf("chat list: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChats", struct{}{}, &raw); err != nil {
		return fmt.Errorf("chat list: %w", err)
	}
	var typed listResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tTITLE\tUPDATED")
		for _, ch := range t.Chats {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", ch.ID, ch.Summary, ch.UpdatedAt)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("chat list: %w", err)
	}
	return nil
}
