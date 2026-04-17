package chat

import (
	"context"
	"fmt"

	"github.com/textql/ana-cli/internal/cli"
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
	fs := newFlagSet("chat share")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	id, err := requirePositionalID("chat share", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	req := shareReq{
		PrimitiveID:   id,
		PrimitiveType: "PRIMITIVE_TYPE_CHAT",
		Channel:       "SHARE_CHANNEL_LINK_COPY",
	}
	if err := c.deps.Unary(ctx, sharingServicePath+"/CreateShare", req, &raw); err != nil {
		return fmt.Errorf("chat share: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed shareResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("chat share: decode response: %w", err)
	}
	if typed.URL != "" {
		fmt.Fprintln(stdio.Stdout, typed.URL)
		return nil
	}
	fmt.Fprintln(stdio.Stdout, typed.Share.ShareToken)
	return nil
}
