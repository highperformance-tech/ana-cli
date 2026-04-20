package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- shared fakes ----------------------------------------------------------

// fakeDeps is the recording test double for Deps. Each Unary call captures
// the path and the JSON-marshaled request body so individual test cases can
// assert on both the endpoint selection and the exact camelCase wire shape.
// Stream is similar: it hands back the caller-supplied fake session and
// records the request for body assertions.
type fakeDeps struct {
	unaryFn   func(ctx context.Context, path string, req, resp any) error
	streamFn  func(ctx context.Context, path string, req any) (StreamSession, error)
	uuidFn    func() string
	lastPath  string
	lastRaw   []byte
	streamPth string
	streamRaw []byte
}

// deps wires the fake into the Deps contract. Each callback routes through
// the zero-cost recording wrapper so tests observe every path.
func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: testcli.RecordUnary(&f.lastPath, &f.lastRaw, f.unaryFn),
		Stream: func(ctx context.Context, path string, req any) (StreamSession, error) {
			f.streamPth = path
			if b, err := json.Marshal(req); err == nil {
				f.streamRaw = b
			}
			if f.streamFn != nil {
				return f.streamFn(ctx, path, req)
			}
			return nil, errors.New("no stream fn configured")
		},
		UUIDFn: f.uuidFn,
	}
}

// fakeStream is an in-memory StreamSession for the send tests. It walks a
// list of pre-built frames (which we round-trip through json.Marshal on the
// way out to exercise the same decode path a real transport uses).
type fakeStream struct {
	frames []map[string]any
	i      int
	err    error // when non-nil, Next returns it immediately
	closed bool
}

func (f *fakeStream) Next(out any) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.i >= len(f.frames) {
		return false, nil
	}
	b, _ := json.Marshal(f.frames[f.i])
	f.i++
	if err := json.Unmarshal(b, out); err != nil {
		return false, err
	}
	return true, nil
}

func (f *fakeStream) Close() error { f.closed = true; return nil }

// --- Group + Help ---------------------------------------------------------

func TestNewReturnsGroupWithAllChildren(t *testing.T) {
	t.Parallel()
	g := New(Deps{})
	if g == nil || g.Children == nil {
		t.Fatalf("nil group")
	}
	for _, name := range []string{
		"new", "list", "show", "history", "send",
		"rename", "bookmark", "unbookmark", "duplicate", "delete", "share",
	} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("empty Summary")
	}
}

func TestHelpAllCommands(t *testing.T) {
	t.Parallel()
	cases := map[string]cli.Command{
		"new":        &newCmd{},
		"list":       &listCmd{},
		"show":       &showCmd{},
		"history":    &historyCmd{},
		"send":       &sendCmd{},
		"rename":     &renameCmd{},
		"bookmark":   &bookmarkCmd{},
		"unbookmark": &unbookmarkCmd{},
		"duplicate":  &duplicateCmd{},
		"delete":     &deleteCmd{},
		"share":      &shareCmd{},
	}
	for name, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", name)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", name, h)
		}
	}
}

// --- helpers (chat.go) ----------------------------------------------------

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: %q", got)
	}
	if got := truncate("hello world", 5); got != "hello" {
		t.Errorf("trim: %q", got)
	}
	if got := truncate("aaaaa", 5); got != "aaaaa" {
		t.Errorf("exact: %q", got)
	}
	// Multi-byte: 5 runes in 6 bytes. The byte-length fast path is a
	// sufficient-only filter (len(s) > n doesn't imply runes > n), so this
	// input falls through to the rune walk, which still returns s unchanged.
	if got := truncate("héllo", 5); got != "héllo" {
		t.Errorf("multibyte-fits: %q", got)
	}
	// Multi-byte truncation: 6 runes in 7 bytes, cap at 3 runes.
	if got := truncate("héllo!", 3); got != "hél" {
		t.Errorf("multibyte-trim: %q", got)
	}
}
