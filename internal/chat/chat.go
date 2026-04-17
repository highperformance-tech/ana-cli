// Package chat provides the `ana chat` verb tree: new/list/show/history/send
// (streaming) plus rename, bookmark/unbookmark, duplicate, delete, and share.
// Like the other verb packages it avoids importing internal/transport and
// internal/config — consumers inject a narrow Deps struct that adapts a real
// transport client to two function fields plus a UUID generator.
package chat

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// chatServicePath is the Connect-RPC prefix every ChatService endpoint uses.
// Centralised so tests can assert against the full path and refactors stay
// mechanical.
const chatServicePath = "/rpc/public/textql.rpc.public.chat.ChatService"

// sharingServicePath is the Connect-RPC prefix for the sharing service, used
// only by the `share` subcommand. Kept in this package so `chat share` does
// not need to reach across to a sibling sharing package just for one call.
const sharingServicePath = "/rpc/public/textql.rpc.public.sharing.SharingService"

// StreamSession is the minimum surface the `send` subcommand needs from a
// streaming RPC call: pull the next frame, decode into an arbitrary value, and
// close the underlying body. transport.StreamReader satisfies this at the call
// site; tests supply an in-memory fake (see chat_test.go).
type StreamSession interface {
	Next(out any) (bool, error)
	Close() error
}

// Deps is the injection boundary for the chat package.
//
//   - Unary JSON-encodes req, POSTs it to path, and JSON-decodes into *resp.
//   - Stream opens a server-streaming call and returns a StreamSession the
//     caller drains frame-by-frame.
//   - UUIDFn returns a fresh v4 UUID string. Injected so tests can assert on
//     a deterministic cellId rather than a random one per run.
type Deps struct {
	Unary  func(ctx context.Context, path string, req, resp any) error
	Stream func(ctx context.Context, path string, req any) (StreamSession, error)
	UUIDFn func() string
}

// New returns the `chat` verb group. Safe to register under any name in the
// root verb table — the group holds no state of its own, only a handful of
// *<verb>Cmd structs that capture the shared Deps.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage chats: new, list, show, history, send (streaming), rename, bookmark, duplicate, delete, share.",
		Children: map[string]cli.Command{
			"new":        &newCmd{deps: deps},
			"list":       &listCmd{deps: deps},
			"show":       &showCmd{deps: deps},
			"history":    &historyCmd{deps: deps},
			"send":       &sendCmd{deps: deps},
			"rename":     &renameCmd{deps: deps},
			"bookmark":   &bookmarkCmd{deps: deps},
			"unbookmark": &unbookmarkCmd{deps: deps},
			"duplicate":  &duplicateCmd{deps: deps},
			"delete":     &deleteCmd{deps: deps},
			"share":      &shareCmd{deps: deps},
		},
	}
}

// newFlagSet returns a FlagSet the way every leaf command wants it: continue
// on parse error (no os.Exit), all output silenced so each command's own
// Help() is the single source of usage text.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags parses args into fs and wraps any error with cli.ErrUsage so the
// root dispatcher maps the failure to exit code 1. Supports flags interspersed
// with positionals: the stdlib FlagSet stops at the first non-flag token, so
// we loop — parse, capture the non-flag, advance past it, parse again — until
// everything is consumed. The flags' final values come from the last pass;
// positionals are reassembled in order and left on fs.Args() for the caller.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var positionals []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
		}
		tail := fs.Args()
		if len(tail) == 0 {
			break
		}
		positionals = append(positionals, tail[0])
		rest = tail[1:]
	}
	// Re-seed fs.Args() with the collected positionals by parsing a synthetic
	// "--" separator. stdlib FlagSet treats everything after "--" as
	// positionals, so this lets callers keep using fs.Args() uniformly. This
	// second Parse cannot fail (inputs post-`--` bypass flag validation).
	if len(positionals) > 0 {
		_ = fs.Parse(append([]string{"--"}, positionals...))
	}
	return nil
}

// usageErrf is the canonical way to emit a user-facing usage error: build a
// message, attach cli.ErrUsage so `errors.Is(err, cli.ErrUsage)` holds and the
// root dispatcher returns exit code 1.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// writeJSON indents a value to w with the 2-space convention used across the
// CLI. A single helper keeps every --json branch byte-identical in output.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal round-trips src through JSON into dst so commands can have one
// Unary call into `map[string]any` and still derive a typed view for table
// rendering without a second RPC.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// requirePositionalID extracts a non-empty UUID-like positional <id> from the
// first arg, returning a usage error otherwise. Chat IDs are UUIDs (strings),
// not integers, so we don't parse them as numbers — the server validates the
// format for us.
func requirePositionalID(verb string, args []string) (string, error) {
	if len(args) == 0 || args[0] == "" {
		return "", usageErrf("%s: <id> positional argument required", verb)
	}
	return args[0], nil
}

// parseConnectorIDs parses a "1,2,3" comma-separated string into []int.
// Whitespace around entries is tolerated; an empty input or any non-integer
// entry is a usage error. Returned slice is guaranteed non-empty on success.
func parseConnectorIDs(raw string) ([]int, error) {
	trim := strings.TrimSpace(raw)
	if trim == "" {
		return nil, usageErrf("--connector: at least one id required")
	}
	parts := strings.Split(trim, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, usageErrf("--connector: empty id in list")
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, usageErrf("--connector: %q is not an integer", p)
		}
		out = append(out, n)
	}
	return out, nil
}

// truncate shortens s to at most n runes. Used by the send renderer to keep
// every streamed line compact regardless of terminal width — we don't probe a
// TTY (stdlib-only) so a fixed cap is the predictable choice.
func truncate(s string, n int) string {
	// Walking runes keeps multi-byte characters whole rather than slicing a
	// UTF-8 sequence mid-way.
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// firstLine returns the first line of s without any trailing newline. Used by
// the send renderer — cells can carry multi-line markdown/code, and dumping a
// whole code block per frame drowns the stream.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
