package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			f.lastReq = req
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
	}
}

func newIO() (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("w boom") }

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	f := &fakeDeps{}
	g := New(f.deps())
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	for _, name := range []string{"show", "stats"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
}

// --- Help() coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"show":  &showCmd{deps: f.deps()},
		"stats": &statsCmd{deps: f.deps()},
	}
	for n, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", n)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", n, h)
		}
	}
}

// --- show ---

func TestShowTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.feed.FeedService/GetFeed" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"posts": []any{
					map[string]any{
						"id": "p1", "title": "KPIs",
						"creatorAgentName": "Org Agent",
						"upvoteCount":      2,
						"createdAt":        "2026-03-31T22:08:16Z",
					},
					// Second post exercises the default-dash branches for
					// title, agent, and created timestamp.
					map[string]any{"id": "p2"},
				},
			}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "TITLE", "AGENT", "UPVOTES", "CREATED", "p1", "KPIs", "Org Agent", "p2", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestShowJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": []any{}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q should start with {", out.String())
	}
	if !strings.Contains(out.String(), "\"posts\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShowUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "feed show") {
		t.Errorf("err=%v", err)
	}
}

func TestShowBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": "nope"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestShowJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": []any{}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// --- stats ---

func TestStatsTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.feed.FeedService/GetFeedStats" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"messagesToday":        116,
				"messagesAllTime":      11518,
				"activeAgents":         3,
				"dashboardsCreated":    7,
				"threadsCreated":       399,
				"playbooksCreated":     5,
				"connectorsConfigured": 4,
				"connectorNames":       []any{"HPT", "tableau.example.com"},
				"activeAgentNames":     []any{"AWS Inspector"},
			}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{
		"messagesToday", "116",
		"messagesAllTime", "11518",
		"activeAgents", "3",
		"dashboardsCreated", "7",
		"threadsCreated", "399",
		"playbooksCreated", "5",
		"connectorsConfigured", "4",
		"connectorNames", "HPT, tableau.example.com",
		"activeAgentNames", "AWS Inspector",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestStatsTableEmptyLists(t *testing.T) {
	// Empty lists should render as "-" so tabwriter keeps alignment.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "connectorNames") || !strings.Contains(out.String(), "-") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestStatsJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": 1}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestStatsUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestStatsBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestStatsRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": "not-a-number"}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestStatsJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": 1}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// --- helpers ---

func TestJoinOrDash(t *testing.T) {
	if got := joinOrDash(nil); got != "-" {
		t.Errorf("nil -> %q", got)
	}
	if got := joinOrDash([]string{"a", "b"}); got != "a, b" {
		t.Errorf("got %q", got)
	}
}
