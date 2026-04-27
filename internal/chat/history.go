package chat

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// historyCmd implements `ana chat history <id>` — GetChatHistory, printing
// cells in capture order (which is chronological per the catalog).
type historyCmd struct{ deps Deps }

func (c *historyCmd) Help() string {
	return "history   Print a chat's messages in chronological order.\n" +
		"Usage: ana chat history <id>"
}

// historyReq mirrors the trivial catalog shape: just `{chatId}`.
type historyReq struct {
	ChatID string `json:"chatId"`
}

// historyResp strips down the `cells` array to the fields the default renderer
// prints. Every cell variant (mdCell/pyCell/statusCell/...) is optional — we
// peek at whichever is populated to derive a single summary line per cell.
type historyResp struct {
	Cells []historyCell `json:"cells"`
}

// historyCell represents one cell from the GetChatHistory response. The rich
// variant blocks are decoded into narrow shapes we can stringify.
type historyCell struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	Lifecycle      string `json:"lifecycle"`
	SenderMemberID string `json:"senderMemberId"`

	MdCell *struct {
		Content string `json:"content"`
	} `json:"mdCell,omitempty"`
	PyCell *struct {
		Code string `json:"code"`
	} `json:"pyCell,omitempty"`
	StatusCell *struct {
		Status string `json:"status"`
	} `json:"statusCell,omitempty"`
	SummaryCell *struct {
		Summary string `json:"summary"`
	} `json:"summaryCell,omitempty"`
}

// kindAndContent returns a `(kind, content)` tuple suitable for one-line
// rendering. Order matters: md > py > status > summary mirrors the webapp's
// own priority when multiple variants somehow appear together.
func (h historyCell) kindAndContent() (string, string) {
	switch {
	case h.MdCell != nil:
		return "md", h.MdCell.Content
	case h.PyCell != nil:
		return "py", h.PyCell.Code
	case h.StatusCell != nil:
		return "status", h.StatusCell.Status
	case h.SummaryCell != nil:
		return "summary", h.SummaryCell.Summary
	default:
		return "other", ""
	}
}

func (c *historyCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("chat history", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("chat history", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChatHistory", historyReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat history: %w", err)
	}
	var typed historyResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *historyResp) error {
		for _, cell := range t.Cells {
			kind, content := cell.kindAndContent()
			// Truncate to 100 chars; matches the send renderer's cap so
			// eyeballing both streams yields aligned column widths.
			line := cli.FirstLine(content)
			if _, err := fmt.Fprintf(w, "[%s] %s %s: %s\n",
				cell.Timestamp, cell.Lifecycle, kind, truncate(line, 100)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("chat history: %w", err)
	}
	return nil
}
