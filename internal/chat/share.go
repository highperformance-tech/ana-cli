package chat

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// shareCmd implements `ana chat share <id>` — POST CreateShare on the sharing
// service, which lives on a different service path than ChatService. The
// command produces a shareable URL (default) or the full response (--json).
type shareCmd struct{ deps Deps }

func (c *shareCmd) Help() string {
	return "share   Create a share link for a chat.\n" +
		"Usage: ana chat share <id>"
}

// shareReq matches the catalog for CreateShare: primitiveId/Type + channel.
// We hard-code primitiveType/channel for chats and LINK_COPY — the only
// combination actually verified end-to-end.
type shareReq struct {
	PrimitiveID   string `json:"primitiveId"`
	PrimitiveType string `json:"primitiveType"`
	Channel       string `json:"channel"`
}

// shareResp carries both the envelope and a top-level `url`. We prefer the
// URL when present, falling back to shareToken for callers piping into
// systems that assemble their own URL.
type shareResp struct {
	Share struct {
		ShareToken string `json:"shareToken"`
	} `json:"share"`
	URL string `json:"url"`
}

func (c *shareCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) > 1 {
		return cli.UsageErrf("chat share: exactly one <id> positional argument required")
	}
	id, err := cli.RequireStringID("chat share", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	req := shareReq{
		PrimitiveID:   id,
		PrimitiveType: "PRIMITIVE_TYPE_CHAT",
		Channel:       "SHARE_CHANNEL_LINK_COPY",
	}
	if err := c.deps.Unary(ctx, sharingServicePath+"/CreateShare", req, &raw); err != nil {
		return fmt.Errorf("chat share: %w", err)
	}
	var typed shareResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *shareResp) error {
		if t.URL != "" {
			_, err := fmt.Fprintln(w, t.URL)
			return err
		}
		_, err := fmt.Fprintln(w, t.Share.ShareToken)
		return err
	}); err != nil {
		return fmt.Errorf("chat share: %w", err)
	}
	return nil
}
