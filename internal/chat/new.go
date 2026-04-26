package chat

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newCmd implements `ana chat new`. The brief describes a flat
// {"connectorIds":[...], "title":"..."} body, but the captured wire shape wraps
// connectorIds inside paradigm.options.universal and has no `title` field at
// all — summaries are derived server-side and renamed later via UpdateChat.
// The catalog wins over the brief per project convention; we follow it here.
type newCmd struct {
	deps  Deps
	ids   []int
	title string
}

func (c *newCmd) Help() string {
	return "new   Create a new chat bound to one or more connectors.\n" +
		"Usage: ana chat new --connector <id[,id...]> [--title <str>]"
}

func (c *newCmd) Flags(fs *flag.FlagSet) {
	fs.Var(cli.IntListFlag(&c.ids, ","), "connector", "connector id(s), comma-separated (required)")
	fs.StringVar(&c.title, "title", "", "optional chat summary/title")
}

// universalOptions mirrors the observed `paradigm.options.universal` block.
// Every downstream flag (sql/python/websearch) is booted true by default, the
// same way the webapp initialises a fresh chat.
type universalOptions struct {
	ConnectorIDs     []int `json:"connectorIds"`
	WebSearchEnabled bool  `json:"webSearchEnabled"`
	SQLEnabled       bool  `json:"sqlEnabled"`
	PythonEnabled    bool  `json:"pythonEnabled"`
}

// paradigmOptions is the polymorphic options holder. Only the universal leaf
// is supported here — feed/etc. aren't part of the v1 CLI surface.
type paradigmOptions struct {
	Universal universalOptions `json:"universal"`
}

// paradigm carries the type+version envelope the server expects alongside the
// typed options oneof.
type paradigm struct {
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Options paradigmOptions `json:"options"`
}

// newReq is the full CreateChat request payload. We omit dashboardMode etc.
// when zero-valued; the server treats missing fields as "defaults".
type newReq struct {
	Paradigm      paradigm `json:"paradigm"`
	Model         string   `json:"model"`
	Research      bool     `json:"research"`
	DashboardMode bool     `json:"dashboardMode"`
	Methodology   string   `json:"methodology"`
	// Summary is the chat title. Catalog does NOT show this in CreateChat —
	// it's only on UpdateChat — but it's harmless extra data when zero-valued
	// (omitempty guards it) and lets --title at create time become an initial
	// summary on servers that honour it.
	Summary string `json:"summary,omitempty"`
}

// newResp picks out the single field we actually print: the new chat's id.
type newResp struct {
	Chat struct {
		ID string `json:"id"`
	} `json:"chat"`
}

func (c *newCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 0 {
		return cli.UsageErrf("chat new: unexpected positional arguments: %v", args)
	}
	if err := cli.RequireFlags(cli.FlagSetFrom(ctx), "chat new", "connector"); err != nil {
		return err
	}
	req := newReq{
		Paradigm: paradigm{
			Type:    "TYPE_UNIVERSAL",
			Version: 1,
			Options: paradigmOptions{
				Universal: universalOptions{
					ConnectorIDs:     c.ids,
					WebSearchEnabled: true,
					SQLEnabled:       true,
					PythonEnabled:    true,
				},
			},
		},
		Model:       "MODEL_DEFAULT",
		Methodology: "METHODOLOGY_ADAPTIVE",
		Summary:     c.title,
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, chatServicePath+"/CreateChat", req, &raw); err != nil {
		return fmt.Errorf("chat new: %w", err)
	}
	var typed newResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *newResp) error {
		_, err := fmt.Fprintln(w, t.Chat.ID)
		return err
	}); err != nil {
		return fmt.Errorf("chat new: %w", err)
	}
	return nil
}
