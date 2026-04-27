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

func TestReportsTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, req, resp any) error {
			if path != playbookServicePath+"/GetPlaybookReports" {
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
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"RUN_ID", "SUBJECT", "RAN_AT", "r1", "Weekly Briefing", "2026-04-13", "r2"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: the r1 row carries a real RFC3339 date
	// full of hyphens, so a bare `strings.Contains(s, "-")` would pass
	// trivially. Locate the r2 row (no subject, no createdAt) and confirm
	// its SUBJECT + RAN_AT cells both render as "-".
	foundR2DashDash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "r2" && fields[1] == "-" && fields[2] == "-" {
			foundR2DashDash = true
			break
		}
	}
	if !foundR2DashDash {
		t.Errorf("expected r2 row to render SUBJECT and RAN_AT as '-': %q", s)
	}
}

func TestReportsJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": []any{}}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"reports\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestReportsRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestReportsRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestReportsMissingID(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestReportsUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestReportsBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"reports", "p1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on bad-flag failure: path=%q", f.lastPath)
	}
}

func TestReportsRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": "nope"}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestReportsJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"reports": []any{}}
			return nil
		},
	}
	cmd := &reportsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
