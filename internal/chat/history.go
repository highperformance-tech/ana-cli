package chat

import (
	"context"
	"fmt"

	"github.com/textql/ana-cli/internal/cli"
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
	fs := newFlagSet("chat history")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("chat history", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChatHistory", historyReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat history: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed historyResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("chat history: decode response: %w", err)
	}
	for _, cell := range typed.Cells {
		kind, content := cell.kindAndContent()
		// Truncate to 100 chars; matches the send renderer's cap so eyeballing
		// both streams yields aligned column widths.
		line := firstLine(content)
		fmt.Fprintf(stdio.Stdout, "[%s] %s %s: %s\n",
			cell.Timestamp, cell.Lifecycle, kind, truncate(line, 100))
	}
	return nil
}
