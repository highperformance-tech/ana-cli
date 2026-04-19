package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
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
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			if b, err := json.Marshal(req); err == nil {
				f.lastRaw = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
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

// newIO builds a cli.IO with in-memory buffers so tests can inspect stdout
// and stderr directly. Env and Now are frozen — the chat commands don't read
// either today but capturing defaults keeps future-proofing cheap.
func newIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

// --- Group + Help ---------------------------------------------------------

func TestNewReturnsGroupWithAllChildren(t *testing.T) {
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

// --- helpers --------------------------------------------------------------

func TestParseConnectorIDs(t *testing.T) {
	if ids, err := parseConnectorIDs("1"); err != nil || len(ids) != 1 || ids[0] != 1 {
		t.Errorf("single: ids=%v err=%v", ids, err)
	}
	if ids, err := parseConnectorIDs(" 1, 2 ,3"); err != nil || len(ids) != 3 {
		t.Errorf("multi: ids=%v err=%v", ids, err)
	}
	if _, err := parseConnectorIDs(""); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("empty should be usage: %v", err)
	}
	if _, err := parseConnectorIDs("   "); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("whitespace-only should be usage: %v", err)
	}
	if _, err := parseConnectorIDs("1,,2"); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("empty entry should be usage: %v", err)
	}
	if _, err := parseConnectorIDs("a,b"); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("non-int should be usage: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short: %q", got)
	}
	if got := truncate("hello world", 5); got != "hello" {
		t.Errorf("trim: %q", got)
	}
	if got := truncate("aaaaa", 5); got != "aaaaa" {
		t.Errorf("exact: %q", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("one\ntwo"); got != "one" {
		t.Errorf("multi: %q", got)
	}
	if got := firstLine("only"); got != "only" {
		t.Errorf("single: %q", got)
	}
}

// --- new ------------------------------------------------------------------

func TestNewHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "new-id"}}
			return nil
		},
	}
	cmd := &newCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"--connector", "1,2", "--title", "hi"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "new-id" {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	for _, w := range []string{
		`"connectorIds":[1,2]`, `"type":"TYPE_UNIVERSAL"`, `"version":1`,
		`"model":"MODEL_DEFAULT"`, `"methodology":"METHODOLOGY_ADAPTIVE"`,
		`"summary":"hi"`,
	} {
		if !strings.Contains(req, w) {
			t.Errorf("req missing %s: %s", w, req)
		}
	}
	if f.lastPath != chatServicePath+"/CreateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestNewOmitTitleWhenEmpty(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return nil }}
	cmd := &newCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"--connector", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRaw), `"summary"`) {
		t.Errorf("summary should be omitted when empty: %s", f.lastRaw)
	}
}

func TestNewJSONBypass(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &newCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"--connector", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestNewMissingConnector(t *testing.T) {
	cmd := &newCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewBadConnector(t *testing.T) {
	cmd := &newCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--connector", "abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewBadFlag(t *testing.T) {
	cmd := &newCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &newCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--connector", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestNewRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	cmd := &newCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--connector", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- list -----------------------------------------------------------------

func TestListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": []any{
				map[string]any{"id": "a", "summary": "first chat", "updatedAt": "2026-04-17"},
				map[string]any{"id": "b", "summary": "second", "updatedAt": "2026-04-16"},
			}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "TITLE") || !strings.Contains(s, "UPDATED") {
		t.Errorf("headers missing: %q", s)
	}
	if !strings.Contains(s, "first chat") || !strings.Contains(s, "second") {
		t.Errorf("rows missing: %q", s)
	}
	if f.lastPath != chatServicePath+"/GetChats" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRaw) != "{}" {
		t.Errorf("body should be empty object: %s", f.lastRaw)
	}
}

func TestListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chats\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestListBadFlag(t *testing.T) {
	cmd := &listCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- show -----------------------------------------------------------------

func TestShowHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{
				"id": "xyz", "summary": "s", "model": "m",
				"updatedAt": "u", "source": "src", "methodology": "x",
			}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"xyz"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, w := range []string{"id: xyz", "title: s", "model: m", "updated: u", "source: src", "methodology: x"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
	if !strings.Contains(string(f.lastRaw), `"chatId":"xyz"`) {
		t.Errorf("req=%s", f.lastRaw)
	}
}

func TestShowJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShowNoChatFallback(t *testing.T) {
	// When `chat` is missing we print raw JSON so the user sees what came back.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": 1.0}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShowMissingPositional(t *testing.T) {
	cmd := &showCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShowBadFlag(t *testing.T) {
	cmd := &showCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShowUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- history --------------------------------------------------------------

func TestHistoryHappy(t *testing.T) {
	// Cover every cell variant branch plus the "other" default.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": []any{
				map[string]any{
					"id": "c1", "timestamp": "t1", "lifecycle": "L",
					"mdCell": map[string]any{"content": "hello\nworld"},
				},
				map[string]any{
					"id": "c2", "timestamp": "t2", "lifecycle": "L",
					"pyCell": map[string]any{"code": "print('hi')"},
				},
				map[string]any{
					"id": "c3", "timestamp": "t3", "lifecycle": "L",
					"statusCell": map[string]any{"status": "working"},
				},
				map[string]any{
					"id": "c4", "timestamp": "t4", "lifecycle": "L",
					"summaryCell": map[string]any{"summary": "done"},
				},
				// No variant → "other".
				map[string]any{"id": "c5", "timestamp": "t5", "lifecycle": "L"},
			}}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, w := range []string{"md: hello", "py: print", "status: working", "summary: done", "other: "} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
	if !strings.Contains(string(f.lastRaw), `"chatId":"x"`) {
		t.Errorf("req=%s", f.lastRaw)
	}
}

func TestHistoryJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": []any{}}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"cells\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHistoryMissingPositional(t *testing.T) {
	cmd := &historyCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryBadFlag(t *testing.T) {
	cmd := &historyCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &historyCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": "not-an-array"}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- send -----------------------------------------------------------------

// sendFake builds a fakeDeps wired to return cellId=X from SendMessage and
// forward Stream calls to the caller-supplied fakeStream.
func sendFake(cellID string, stream *fakeStream) (*fakeDeps, StreamSession) {
	f := &fakeDeps{
		uuidFn: func() string { return cellID },
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*sendMessageResp)
			out.CellID = cellID
			return nil
		},
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return stream, nil
		},
	}
	return f, stream
}

func TestSendHappyPath(t *testing.T) {
	// Target cell X progresses through three lifecycles and terminates the
	// loop at EXECUTED. The wait-all flag is OFF so only X's executed event
	// matters — intermediate frames from other cells would not block us.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "hello"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "more"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "final"}},
		// Extra frame past EXECUTED should never be consumed.
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "tail"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-id", "hello?"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream should be closed")
	}
	if stream.i != 3 {
		t.Errorf("consumed frames=%d, want 3", stream.i)
	}
	if !strings.Contains(out.String(), "LIFECYCLE_EXECUTED") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRaw), `"messageId":"X"`) {
		t.Errorf("messageId not wired through: %s", f.lastRaw)
	}
	if f.streamPth != chatServicePath+"/StreamChat" {
		t.Errorf("stream path=%s", f.streamPth)
	}
	if !strings.Contains(string(f.streamRaw), `"chatId":"chat-id"`) {
		t.Errorf("stream body=%s", f.streamRaw)
	}
}

func TestSendWaitAll(t *testing.T) {
	// Two cells; must not exit until BOTH have EXECUTED. Interleave so the
	// target cell reaches EXECUTED before Y does — with --wait-all we keep
	// reading until Y also EXECUTEs.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "pyCell": map[string]any{"code": "code"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "statusCell": map[string]any{"status": "s"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "pyCell": map[string]any{"code": "code"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_EXECUTED", "statusCell": map[string]any{"status": "s"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "hi", "--wait-all"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if stream.i != 4 {
		t.Errorf("wait-all should drain all 4 frames, got %d", stream.i)
	}
}

func TestSendNaturalEndOfStream(t *testing.T) {
	// EXECUTED never arrives; stream runs out cleanly → nil return.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "a"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "b"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio); err != nil {
		t.Errorf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream not closed on natural end")
	}
}

func TestSendStreamError(t *testing.T) {
	stream := &fakeStream{err: errors.New("mid-way boom")}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "mid-way boom") {
		t.Errorf("err=%v", err)
	}
	if !stream.closed {
		t.Errorf("stream not closed on error (defer missed)")
	}
}

func TestSendMessageError(t *testing.T) {
	// SendMessage fails → no Stream call at all.
	f := &fakeDeps{
		uuidFn:  func() string { return "X" },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("send-boom") },
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return nil, errors.New("should not be called")
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "send-boom") {
		t.Errorf("err=%v", err)
	}
	if f.streamPth != "" {
		t.Errorf("stream should not have been opened")
	}
}

func TestSendStreamOpenError(t *testing.T) {
	// SendMessage fine, but opening the stream itself fails.
	f := &fakeDeps{
		uuidFn:  func() string { return "X" },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return nil },
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return nil, errors.New("open-boom")
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "open-boom") {
		t.Errorf("err=%v", err)
	}
}

// Regression: positionals BEFORE trailing flag must not drop the flag. The
// stdlib fs.Parse stops at the first non-flag token, so the naive
// implementation would silently ignore --wait-all here. 100% coverage on the
// central cli.ParseFlags helper didn't catch the prod bug because the verb
// wrapper was bypassing it — this explicit test locks in the verb-level path.
func TestSendRegressionPositionalBeforeFlags(t *testing.T) {
	// --wait-all appears AFTER both positionals; we must read past X's
	// EXECUTED until Y also EXECUTEs. If the flag were dropped the loop would
	// exit after frame 3 (X EXECUTED) and we'd consume fewer frames.
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "a"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_CREATED", "mdCell": map[string]any{"content": "b"}},
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "c"}},
		{"id": "Y", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "d"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-id", "hello?", "--wait-all"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if stream.i != 4 {
		t.Errorf("--wait-all dropped when placed after positionals: consumed=%d want=4", stream.i)
	}
}

func TestSendBadFlag(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendMissingID(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendNoMessage(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-id"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendMessageFilePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(p, []byte("hello from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "ok"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "--message-file", p}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRaw), `"message":"hello from file"`) {
		t.Errorf("body=%s", f.lastRaw)
	}
}

func TestSendMessageFileMissing(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "--message-file", "/nope/does-not-exist-12345"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read message file") {
		t.Errorf("err=%v", err)
	}
}

func TestSendMessageFileStdin(t *testing.T) {
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTED", "mdCell": map[string]any{"content": "ok"}},
	}}
	f, _ := sendFake("X", stream)
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader("from-stdin-input"))
	if err := cmd.Run(context.Background(), []string{"c", "--message-file", "-"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRaw), `"message":"from-stdin-input"`) {
		t.Errorf("body=%s", f.lastRaw)
	}
}

func TestSendStdinEmpty(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"c", "--message-file", "-"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// errReader forces the ReadAll branch of stdin reading to error out.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestSendStdinReadErr(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(errReader{err: errors.New("read-fail")})
	err := cmd.Run(context.Background(), []string{"c", "--message-file", "-"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read-fail") {
		t.Errorf("err=%v", err)
	}
}

func TestSendStdinNilReader(t *testing.T) {
	// Direct helper call — the run path always has a non-nil Stdin because
	// cli.IO sets os.Stdin at root. This exercises the nil branch.
	if _, err := resolveMessage("", "-", nil); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("want usage err: %v", err)
	}
}

func TestSendBothPositionalAndFile(t *testing.T) {
	cmd := &sendCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"c", "positional", "--message-file", "/tmp/anything"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSendUUIDFnNil(t *testing.T) {
	// With nil UUIDFn the messageId is empty; server's CellID response becomes
	// the authoritative target. Cover the "CellID empty → fall back to msgID"
	// branch by having the stub return empty and asserting we still proceed
	// (the fallback yields "" as target, which won't match any cell; stream
	// ends naturally).
	stream := &fakeStream{frames: []map[string]any{
		{"id": "X", "lifecycle": "LIFECYCLE_EXECUTING", "mdCell": map[string]any{"content": "mid"}},
	}}
	f := &fakeDeps{
		uuidFn: nil, // explicit
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*sendMessageResp)
			out.CellID = "" // server returns empty → fallback triggers
			return nil
		},
		streamFn: func(_ context.Context, _ string, _ any) (StreamSession, error) {
			return stream, nil
		},
	}
	cmd := &sendCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"c", "hi"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
}

// --- simple: rename ------------------------------------------------------

func TestRenameHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &renameCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-x", "new title"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "ok" {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	if !strings.Contains(req, `"chatId":"chat-x"`) || !strings.Contains(req, `"summary":"new title"`) {
		t.Errorf("req=%s", req)
	}
	if f.lastPath != chatServicePath+"/UpdateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestRenameMissingID(t *testing.T) {
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameMissingTitle(t *testing.T) {
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-x"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameEmptyTitle(t *testing.T) {
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-x", ""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameBadFlag(t *testing.T) {
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &renameCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x", "t"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestRenameJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &renameCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x", "t"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- simple: bookmark / unbookmark ---------------------------------------

func TestBookmarkHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &bookmarkCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "ok" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/BookmarkChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestBookmarkMissingID(t *testing.T) {
	cmd := &bookmarkCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestBookmarkBadFlag(t *testing.T) {
	cmd := &bookmarkCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestBookmarkUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &bookmarkCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestBookmarkJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"foo": 1.0}
			return nil
		},
	}
	cmd := &bookmarkCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"foo\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestUnbookmarkHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &unbookmarkCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.lastPath != chatServicePath+"/UnbookmarkChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

// --- simple: delete -------------------------------------------------------

func TestDeleteHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-7"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted chat-7") {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/DeleteChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDeleteMissingID(t *testing.T) {
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteBadFlag(t *testing.T) {
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &deleteCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "{}") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- simple: duplicate ---------------------------------------------------

func TestDuplicateHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "dup-id"}}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"src"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "dup-id" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/DuplicateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDuplicateMissingID(t *testing.T) {
	cmd := &duplicateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateBadFlag(t *testing.T) {
	cmd := &duplicateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- share ---------------------------------------------------------------

func TestShareHappyURL(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"share": map[string]any{"shareToken": "TKN"},
				"url":   "https://app/chat/abc?ref=TKN",
			}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "https://app/chat/abc?ref=TKN") {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	for _, w := range []string{
		`"primitiveId":"abc"`, `"primitiveType":"PRIMITIVE_TYPE_CHAT"`,
		`"channel":"SHARE_CHANNEL_LINK_COPY"`,
	} {
		if !strings.Contains(req, w) {
			t.Errorf("req missing %q: %s", w, req)
		}
	}
	if f.lastPath != sharingServicePath+"/CreateShare" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestShareTokenFallback(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": map[string]any{"shareToken": "TKN"}}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(context.Background(), []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "TKN" {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShareJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": map[string]any{"shareToken": "T"}}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(nil)
	if err := cmd.Run(ctx, []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"share\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShareMissingID(t *testing.T) {
	cmd := &shareCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShareBadFlag(t *testing.T) {
	cmd := &shareCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShareUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &shareCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestShareRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": "not-an-object"}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, _, _ := newIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// TestFrameContentAllVariants exercises every branch of frameContent. The send
// renderer picks md > py > status > summary > playbook; we construct a frame
// per variant (minus the leaders already covered in streaming tests) plus the
// default empty case so the method reaches 100%.
func TestFrameContentAllVariants(t *testing.T) {
	cases := []struct {
		name string
		f    streamFrame
		want string
	}{
		{"md", streamFrame{MdCell: &struct {
			Content string `json:"content"`
		}{Content: "m"}}, "m"},
		{"py", streamFrame{PyCell: &struct {
			Code string `json:"code"`
		}{Code: "c"}}, "c"},
		{"status", streamFrame{StatusCell: &struct {
			Status string `json:"status"`
		}{Status: "s"}}, "s"},
		{"summary", streamFrame{SummaryCell: &struct {
			Summary string `json:"summary"`
		}{Summary: "su"}}, "su"},
		{"playbook", streamFrame{PlaybookEditorCell: &struct {
			Action string `json:"action"`
		}{Action: "a"}}, "a"},
		{"empty", streamFrame{}, ""},
	}
	for _, tc := range cases {
		if got := tc.f.frameContent(); got != tc.want {
			t.Errorf("%s: got=%q want=%q", tc.name, got, tc.want)
		}
	}
}
