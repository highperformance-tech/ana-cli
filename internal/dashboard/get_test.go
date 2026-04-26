package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestGetSummary(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboard": map[string]any{
					"id":        "d1",
					"name":      "HPT",
					"orgId":     "o1",
					"creatorId": "c1",
					"code":      "print(1)",
				},
			}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"id:", "name:", "HPT", "orgId:", "creatorId:", "code:", "8 bytes"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q: %q", want, s)
		}
	}
	if !strings.Contains(string(f.lastRawReq), `"dashboardId":"d1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/GetDashboard" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestGetJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboard": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboard\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestGetNoDashboardKeyFallback(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": 1.0}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestGetRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestGetRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestGetMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &getCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestGetUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"get", "d-1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestGetWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboard": map[string]any{"id": "x", "name": "n"}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	err := cmd.Run(context.Background(), []string{"x"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "dashboard get") {
		t.Errorf("err=%v", err)
	}
}
