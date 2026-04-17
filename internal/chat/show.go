package chat

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// showCmd implements `ana chat show <id>` — GetChat with `{chatId: "..."}`.
// Default output is a compact summary (id/title/updated/model/source); --json
// prints the full raw response.
type showCmd struct{ deps Deps }

func (c *showCmd) Help() string {
	return "show   Show a chat's summary (default) or full JSON (--json).\n" +
		"Usage: ana chat show <id>"
}

// showReq is the exact wire shape — catalog confirms a single `chatId` field.
type showReq struct {
	ChatID string `json:"chatId"`
}

// showResp is the compact typed projection. We only pull the fields the
// summary view actually prints; everything else is available via --json.
type showResp struct {
	Chat struct {
		ID          string `json:"id"`
		Summary     string `json:"summary"`
		Model       string `json:"model"`
		UpdatedAt   string `json:"updatedAt"`
		Source      string `json:"source"`
		Methodology string `json:"methodology"`
	} `json:"chat"`
}

func (c *showCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("chat show")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("chat show", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChat", showReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat show: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed showResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("chat show: decode response: %w", err)
	}
	// A missing `chat` envelope falls through to --json so the user sees the
	// response shape rather than a block of empty fields.
	if typed.Chat.ID == "" {
		return writeJSON(stdio.Stdout, raw)
	}
	fmt.Fprintf(stdio.Stdout, "id: %s\n", typed.Chat.ID)
	fmt.Fprintf(stdio.Stdout, "title: %s\n", typed.Chat.Summary)
	fmt.Fprintf(stdio.Stdout, "model: %s\n", typed.Chat.Model)
	fmt.Fprintf(stdio.Stdout, "updated: %s\n", typed.Chat.UpdatedAt)
	fmt.Fprintf(stdio.Stdout, "source: %s\n", typed.Chat.Source)
	fmt.Fprintf(stdio.Stdout, "methodology: %s\n", typed.Chat.Methodology)
	return nil
}
