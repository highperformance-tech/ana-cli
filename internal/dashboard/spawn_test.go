package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestSpawnHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"refreshedAt": "2026-04-16T16:00:18Z"}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "spawned d1") || !strings.Contains(out.String(), "2026-04-16T16:00:18Z") {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != servicePath+"/SpawnDashboard" {
		t.Errorf("path=%s", f.lastPath)
	}
	if !strings.Contains(string(f.lastRawReq), `"dashboardId":"d1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSpawnJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"refreshedAt": "t"}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"refreshedAt\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSpawnNoRefreshedAtFallback(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"weird": true}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"weird\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestSpawnRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestSpawnRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestSpawnMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &spawnCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSpawnUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &spawnCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSpawnBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"spawn", "d-1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSpawnWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"refreshedAt": "t"}
			return nil
		},
	}
	cmd := &spawnCmd{deps: f.deps()}
	err := cmd.Run(context.Background(), []string{"d1"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "dashboard spawn") {
		t.Errorf("err=%v", err)
	}
}
