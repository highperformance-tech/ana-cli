package playbook

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

// fakeDeps is the table-driven fake for Deps. Each call's path and
// JSON-encoded request body are recorded so assertions can inspect the
// wire-level payload the command produced.
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

// deps returns a Deps whose Unary funnels through the fake so tests can
// assert on recorded inputs after the command runs.
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

// newIO builds a cli.IO with in-memory streams so tests can assert on output
// without touching the real file descriptors.
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

// failingWriter returns err on every Write so we can trip json.Encoder paths.
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
	for _, name := range []string{"list", "get", "reports", "lineage"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
}

// --- Help() text coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"list":    &listCmd{deps: f.deps()},
		"get":     &getCmd{deps: f.deps()},
		"reports": &reportsCmd{deps: f.deps()},
		"lineage": &lineageCmd{deps: f.deps()},
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

// --- list ---

func TestListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.playbook.PlaybookService/GetPlaybooks" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"playbooks": []any{
					map[string]any{"id": "pb1", "name": "Weekly", "cronString": "0 13 * * 1"},
					map[string]any{"id": "pb2", "name": "Ad hoc"},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "SCHEDULE", "pb1", "Weekly", "0 13 * * 1", "pb2", "Ad hoc", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbooks": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"playbooks\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	boom := errors.New("net boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "playbook list") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbooks": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- get ---

func TestGetTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, req, resp any) error {
			if path != "/rpc/public/textql.rpc.public.playbook.PlaybookService/GetPlaybook" {
				t.Errorf("path=%s", path)
			}
			// Spot-check the wire shape: request must serialise to camelCase.
			b, _ := json.Marshal(req)
			if !strings.Contains(string(b), "\"playbookId\"") {
				t.Errorf("req=%s missing playbookId", string(b))
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"playbook": map[string]any{
					"id":                "pb1",
					"name":              "Weekly Cash Flow",
					"status":            "STATUS_ACTIVE",
					"triggerType":       "TRIGGER_TYPE_CRON",
					"cronString":        "0 13 * * 1",
					"paradigmType":      "TYPE_UNIVERSAL",
					"reportOutputStyle": "CONCISE",
					"latestChatId":      "chat-1",
					"createdAt":         "2026-03-27T00:55:12Z",
					"updatedAt":         "2026-03-31T17:00:42Z",
					"owner":             map[string]any{"memberEmail": "owner@example.com"},
				},
			}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{
		"id", "pb1", "name", "Weekly Cash Flow",
		"status", "STATUS_ACTIVE",
		"triggerType", "TRIGGER_TYPE_CRON",
		"cronString", "0 13 * * 1",
		"paradigmType", "TYPE_UNIVERSAL",
		"reportOutputStyle", "CONCISE",
		"owner", "owner@example.com",
		"latestChatId", "chat-1",
		"createdAt", "2026-03-27",
		"updatedAt", "2026-03-31",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestGetJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": map[string]any{"id": "pb1"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"playbook\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetMissingID(t *testing.T) {
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": "not-an-object"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestGetEmptyPlaybookFallsBackToJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// No `playbook` envelope or empty id — fall through to --json.
			*out = map[string]any{"other": "x"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q want raw JSON fallback", out.String())
	}
}

// --- reports ---

func TestReportsTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, req, resp any) error {
			if path != "/rpc/public/textql.rpc.public.playbook.PlaybookService/GetPlaybookReports" {
				t.Errorf("path=%s", path)
			}
			b, _ := json.Marshal(req)
			if !strings.Contains(string(b), "\"playbookId\":\"pb1\"") {
				t.Errorf("req=%s", string(b))
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"reports": []any{
					map[string]any{"id": "r1", "subject": "Weekly Briefing", "createdAt": "2026-04-13T13:11:25Z"},
					map[string]any{"id": "r2"},
				},
			}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"RUN_ID", "STATUS", "RAN_AT", "r1", "Weekly Briefing", "2026-04-13", "r2", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestReportsJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": []any{}}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"reports\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestReportsMissingID(t *testing.T) {
	f := &fakeDeps{}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestReportsUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestReportsBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestReportsRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": "nope"}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- lineage ---

func TestLineageEmptyResponse(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.playbook.PlaybookService/GetPlaybookLineage" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "no lineage edges") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageEdgesTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"edges": []any{
					map[string]any{"from": "a", "to": "b", "type": "depends_on"},
					map[string]any{"source": "c", "target": "d"},
					map[string]any{},
				},
			}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"FROM", "TO", "TYPE", "a", "b", "depends_on", "c", "d", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestLineageLineageFallback(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"lineage": []any{
					map[string]any{"from": "x", "to": "y"},
				},
			}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "x") || !strings.Contains(out.String(), "y") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageNodesFallback(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"nodes": []any{
					map[string]any{"source": "n1", "target": "n2", "type": "flows_to"},
				},
			}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{"n1", "n2", "flows_to"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in output %q", want, out.String())
		}
	}
}

func TestLineageJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": []any{}}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"edges\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageMissingID(t *testing.T) {
	f := &fakeDeps{}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestLineageUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestLineageBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLineageRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": "nope"}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- --json failing writer paths, one per command, for full coverage ---

func TestListJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbooks": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

func TestGetJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": map[string]any{"id": "pb1"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

func TestReportsJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": []any{}}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

func TestLineageJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": []any{}}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// getCmd's fall-through-to-JSON path with a failing writer — trips writeJSON
// from the non-JSON branch.
func TestGetEmptyPlaybookFallbackJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": "x"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
