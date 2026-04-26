package playbook

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestGetTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, req, resp any) error {
			if path != playbookServicePath+"/GetPlaybook" {
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
	stdio, out, _ := testcli.NewIO(nil)
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
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": map[string]any{"id": "pb1"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"playbook\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestGetRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestGetRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetMissingID(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"get", "p1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": "not-an-object"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestGetEmptyPlaybookFallsBackToJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// No `playbook` envelope or empty id — fall through to --json.
			*out = map[string]any{"other": "x"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q want raw JSON fallback", out.String())
	}
}

func TestGetJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"playbook": map[string]any{"id": "pb1"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// getCmd's fall-through-to-JSON path with a failing writer — trips writeJSON
// from the non-JSON branch.
func TestGetEmptyPlaybookFallbackJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": "x"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio := testcli.FailingIO()
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
