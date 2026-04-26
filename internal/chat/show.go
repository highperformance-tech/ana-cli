package chat

import (
	"context"
	"fmt"
	"io"

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
	id, err := cli.RequireStringID("chat show", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/GetChat", showReq{ChatID: id}, &raw); err != nil {
		return fmt.Errorf("chat show: %w", err)
	}
	var typed showResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *showResp) error {
		// A missing `chat` envelope falls through to --json so the user sees
		// the response shape rather than a block of empty fields.
		if t.Chat.ID == "" {
			return cli.WriteJSON(w, raw)
		}
		_, err := fmt.Fprintf(w, "id: %s\ntitle: %s\nmodel: %s\nupdated: %s\nsource: %s\nmethodology: %s\n",
			t.Chat.ID, t.Chat.Summary, t.Chat.Model, t.Chat.UpdatedAt, t.Chat.Source, t.Chat.Methodology)
		return err
	}); err != nil {
		return fmt.Errorf("chat show: %w", err)
	}
	return nil
}
