package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	now        time.Time
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

// deps returns a Deps whose Unary records every call and whose Now returns
// f.now so --since produces a deterministic request body.
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
		Now: func() time.Time { return f.now },
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

func TestNewReturnsGroupWithTailChild(t *testing.T) {
	f := &fakeDeps{}
	g := New(f.deps())
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	if _, ok := g.Children["tail"]; !ok {
		t.Errorf("missing tail child")
	}
}

// A zero Deps.Now should be defaulted to time.Now by New.
func TestNewDefaultsNow(t *testing.T) {
	g := New(Deps{Unary: func(context.Context, string, any, any) error { return nil }})
	if g == nil {
		t.Fatalf("New returned nil")
	}
	// Fish out the tail command and check its Deps.Now is non-nil.
	tc, ok := g.Children["tail"].(*tailCmd)
	if !ok {
		t.Fatalf("tail child not *tailCmd")
	}
	if tc.deps.Now == nil {
		t.Errorf("Now not defaulted")
	}
}

// --- Help() ---

func TestHelpNonEmpty(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	h := cmd.Help()
	if h == "" || !strings.Contains(strings.ToLower(h), "usage") {
		t.Errorf("help=%q", h)
	}
}

// --- tail: happy path / table ---

func TestTailTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.audit_log.AuditLogService/ListAuditLogs" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"entries": []any{
					map[string]any{
						"actorEmail":   "brad@example.com",
						"action":       "api_key.created",
						"resourceType": "api_key",
						"resourceId":   "f8e934f5",
						"createdAt":    "2026-04-17T15:19:15Z",
					},
					// resourceId-only (no type).
					map[string]any{
						"actorEmail": "ken@example.com",
						"action":     "auth.login_success",
						"resourceId": "r1",
						"createdAt":  "2026-04-14T20:04:35Z",
					},
					// No resource at all — target should be "-".
					map[string]any{
						"actorEmail": "ken@example.com",
						"action":     "auth.logout",
						"createdAt":  "2026-04-14T20:05:00Z",
					},
					// Everything missing — all default to "-".
					map[string]any{},
				},
			}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{
		"TIME", "ACTOR", "ACTION", "TARGET",
		"brad@example.com", "api_key.created", "api_key:f8e934f5",
		"ken@example.com", "auth.login_success", "r1",
		"auth.logout", "-",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// No flags -> body must be exactly {}.
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

// --- tail: --since with fake clock ---

func TestTailSinceSetsRFC3339(t *testing.T) {
	// A fixed non-UTC instant. The code converts to UTC before formatting so
	// we can assert the exact wire string regardless of the local zone.
	f := &fakeDeps{now: time.Date(2026, 4, 17, 10, 0, 0, 0, time.FixedZone("EST", -5*3600))}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"--since", "1h"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	wantSince := f.now.Add(-time.Hour).UTC().Format(time.RFC3339)
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	if got["since"] != wantSince {
		t.Errorf("since=%v want %q", got["since"], wantSince)
	}
	if _, ok := got["limit"]; ok {
		t.Errorf("body should not contain limit when --limit is unset: %s", f.lastRawReq)
	}
}

func TestTailSinceInvalid(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--since", "nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// --- tail: --limit pass-through ---

func TestTailLimitZeroOmitted(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"--limit", "0"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestTailLimitSet(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"--limit", "50"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	// JSON numbers decode to float64.
	if v, ok := got["limit"].(float64); !ok || int(v) != 50 {
		t.Errorf("limit=%v want 50", got["limit"])
	}
}

// --- tail: no --since -> body omits "since" ---

func TestTailNoSinceOmitsKey(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRawReq), "since") {
		t.Errorf("req should omit since: %s", f.lastRawReq)
	}
}

// --- tail: --json ---

func TestTailJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": []any{}}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q should start with {", out.String())
	}
	if !strings.Contains(out.String(), "\"entries\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- tail: error paths ---

func TestTailUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "audit tail") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestTailBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestTailRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": "nope"}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestTailJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": []any{}}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// --- helpers ---

func TestWriteJSONErr(t *testing.T) {
	if err := writeJSON(io.Discard, make(chan int)); err == nil {
		t.Errorf("want error on unsupported value")
	}
}

func TestWriteJSONOK(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]string{"k": "v"}); err != nil {
		t.Errorf("err=%v", err)
	}
	if !strings.Contains(buf.String(), "\"k\"") {
		t.Errorf("out=%q", buf.String())
	}
}

func TestRemarshalMarshalErr(t *testing.T) {
	if err := remarshal(make(chan int), &struct{}{}); err == nil {
		t.Errorf("want error on unsupported source")
	}
}

func TestRemarshalOK(t *testing.T) {
	src := map[string]any{"x": 1}
	var dst struct {
		X int `json:"x"`
	}
	if err := remarshal(src, &dst); err != nil {
		t.Errorf("err=%v", err)
	}
	if dst.X != 1 {
		t.Errorf("dst=%+v", dst)
	}
}

func TestUsageErrf(t *testing.T) {
	err := usageErrf("%s needs %s", "foo", "bar")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("not a usage error: %v", err)
	}
	if !strings.Contains(err.Error(), "foo needs bar") {
		t.Errorf("err=%q", err.Error())
	}
}
