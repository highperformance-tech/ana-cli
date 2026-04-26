package chat

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// sendCmd implements `ana chat send <id> <message>` — the only streaming verb.
// It first POSTs SendMessage to get the bot's target cellId, then opens
// StreamChat and renders incoming frames one line each until the target cell
// (or, with --wait-all, every cell ever seen) reaches LIFECYCLE_EXECUTED.
type sendCmd struct {
	deps        Deps
	messageFile string
	waitAll     bool
}

func (c *sendCmd) Help() string {
	return "send   Send a message to a chat and stream the response.\n" +
		"Usage: ana chat send <id> <message> | --message-file PATH | --message-file -  [--wait-all]"
}

func (c *sendCmd) Flags(fs *flag.FlagSet) {
	fs.StringVar(&c.messageFile, "message-file", "", "read message from PATH (or - for stdin)")
	fs.BoolVar(&c.waitAll, "wait-all", false, "wait for ALL cells to reach LIFECYCLE_EXECUTED (default: just ours)")
}

// sendMessageReq matches the captured SendMessage wire shape 1:1. `messageId`
// is generated client-side and echoed back as the server's `cellId` so we can
// reliably know which streamed cell is ours.
type sendMessageReq struct {
	ChatID    string `json:"chatId"`
	Message   string `json:"message"`
	MessageID string `json:"messageId"`
}

// sendMessageResp picks out just `cellId`. The server stamps this equal to the
// messageId we supplied, but we decode defensively from the response rather
// than assuming they match — if that ever drifts we'd rather key the render
// loop on the server's authoritative value.
type sendMessageResp struct {
	CellID string `json:"cellId"`
}

// streamChatReq is what we post to /StreamChat to kick off the server stream.
// The catalog shows `latestCompleteCellId` as common (helps the server skip
// cells the client has already seen); we pass the cellId we just got from
// SendMessage so the stream starts at the right point.
type streamChatReq struct {
	ChatID               string `json:"chatId"`
	LatestCompleteCellID string `json:"latestCompleteCellId"`
	Research             bool   `json:"research"`
	Model                string `json:"model"`
}

// streamFrame is the minimal view of a streamed frame that the renderer
// needs. Real frames carry one of many variant blocks (mdCell/pyCell/...); we
// decode a tiny projection of each so `cli.FirstLine(content)` gives us something
// readable no matter which variant is present.
type streamFrame struct {
	ID        string `json:"id"`
	Lifecycle string `json:"lifecycle"`

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
	PlaybookEditorCell *struct {
		Action string `json:"action"`
	} `json:"playbookEditorCell,omitempty"`
}

// frameContent returns the best string to display for a frame, preferring
// markdown over code over status over summary over playbook-action.
func (f streamFrame) frameContent() string {
	switch {
	case f.MdCell != nil:
		return f.MdCell.Content
	case f.PyCell != nil:
		return f.PyCell.Code
	case f.StatusCell != nil:
		return f.StatusCell.Status
	case f.SummaryCell != nil:
		return f.SummaryCell.Summary
	case f.PlaybookEditorCell != nil:
		return f.PlaybookEditorCell.Action
	default:
		return ""
	}
}

// resolveMessage merges the three mutually-exclusive message sources into a
// single string: positional arg, --message-file PATH, or --message-file -
// (stdin). Exactly one must be in play; anything else is a usage error.
func resolveMessage(positional string, messageFile string, stdin io.Reader) (string, error) {
	switch {
	case messageFile != "" && positional != "":
		return "", cli.UsageErrf("chat send: cannot combine positional <message> and --message-file")
	case messageFile == "-":
		if stdin == nil {
			return "", cli.UsageErrf("chat send: --message-file - requires stdin")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("chat send: read stdin: %w", err)
		}
		if len(b) == 0 {
			return "", cli.UsageErrf("chat send: --message-file - read empty input")
		}
		return string(b), nil
	case messageFile != "":
		b, err := os.ReadFile(messageFile)
		if err != nil {
			return "", fmt.Errorf("chat send: read message file: %w", err)
		}
		return string(b), nil
	case positional != "":
		return positional, nil
	default:
		return "", cli.UsageErrf("chat send: <message> or --message-file required")
	}
}

func (c *sendCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	id, err := cli.RequireStringID("chat send", args)
	if err != nil {
		return err
	}
	if len(args) > 2 {
		return cli.UsageErrf("chat send: exactly one positional <message> is allowed; quote messages with spaces or use --message-file")
	}
	positional := ""
	if len(args) >= 2 {
		positional = args[1]
	}
	msg, err := resolveMessage(positional, c.messageFile, stdio.Stdin)
	if err != nil {
		return err
	}

	// messageId is generated client-side. The server echoes it back as cellId,
	// so injecting UUIDFn keeps tests deterministic without relying on
	// whatever the server decides to return (which in fakes we also control).
	msgID := ""
	if c.deps.UUIDFn != nil {
		msgID = c.deps.UUIDFn()
	}

	var sendResp sendMessageResp
	sendReq := sendMessageReq{ChatID: id, Message: msg, MessageID: msgID}
	if err := c.deps.Unary(ctx, chatServicePath+"/SendMessage", sendReq, &sendResp); err != nil {
		return fmt.Errorf("chat send: SendMessage: %w", err)
	}
	targetCell := sendResp.CellID
	if targetCell == "" {
		// Fall back to the messageId we supplied — the webapp treats these
		// two values as interchangeable, and the catalog confirms they match.
		targetCell = msgID
	}

	sess, err := c.deps.Stream(ctx, chatServicePath+"/StreamChat", streamChatReq{
		ChatID:               id,
		LatestCompleteCellID: targetCell,
		Research:             false,
		Model:                "MODEL_DEFAULT",
	})
	if err != nil {
		return fmt.Errorf("chat send: StreamChat: %w", err)
	}
	// Deferred close is belt-and-suspenders: we also call Close on the happy
	// path before returning so test assertions on `closed` fire regardless of
	// whether the loop exited via EXECUTED or natural EOF.
	defer sess.Close()

	executed := make(map[string]bool) // cellId -> saw LIFECYCLE_EXECUTED
	seen := make(map[string]struct{}) // cellId -> ever rendered (set semantics)

	for {
		var frame streamFrame
		ok, err := sess.Next(&frame)
		if err != nil {
			return fmt.Errorf("chat send: stream: %w", err)
		}
		if !ok {
			// Natural end of stream before our terminal condition — not an
			// error (the server just ran out of frames).
			return nil
		}
		content := frame.frameContent()
		fmt.Fprintf(stdio.Stdout, "[%s] %s: %s\n",
			frame.Lifecycle, frame.ID, truncate(cli.FirstLine(content), 100))

		if frame.ID != "" {
			seen[frame.ID] = struct{}{}
			if frame.Lifecycle == "LIFECYCLE_EXECUTED" {
				executed[frame.ID] = true
			}
		}

		if c.waitAll {
			// Terminate only when every cellId we've rendered has at least
			// one EXECUTED frame. Edge case: zero seen means we must keep
			// reading (happens if the server opens the stream with non-cell
			// metadata frames; not observed, but cheap to be safe about).
			if len(seen) > 0 && allExecuted(seen, executed) {
				return nil
			}
		} else if executed[targetCell] {
			return nil
		}
	}
}

// allExecuted returns true when every key in seen has a true value in
// executed. Extracted so the loop body stays flat and the tests can prod it
// directly if needed via a public-enough name within the package.
func allExecuted(seen map[string]struct{}, executed map[string]bool) bool {
	for id := range seen {
		if !executed[id] {
			return false
		}
	}
	return true
}
