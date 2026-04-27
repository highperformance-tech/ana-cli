package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestHealthHealthy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{
						"dashboardId":  "d1",
						"status":       "HEALTH_STATUS_HEALTHY",
						"streamlitUrl": "x:8501",
						"embedUrl":     "/sandbox/proxy/x/8501/",
					},
				},
			}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "d1 HEALTHY") || !strings.Contains(s, "streamlitUrl: x:8501") || !strings.Contains(s, "embedUrl: /sandbox/proxy/x/8501/") {
		t.Errorf("stdout=%q", s)
	}
	if f.lastPath != servicePath+"/CheckDashboardHealth" {
		t.Errorf("path=%s", f.lastPath)
	}
	// Catalog-shape check: wire body must be plural + array.
	if !strings.Contains(string(f.lastRawReq), `"dashboardIds":["d1"]`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestHealthUnhealthyWithMessage(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{
						"dashboardId": "d1",
						"status":      "HEALTH_STATUS_UNHEALTHY",
						"message":     "container crashed",
					},
				},
			}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "d1 UNHEALTHY: container crashed") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHealthUnknownAndCustomStatusLabels(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                          "UNKNOWN",
		"HEALTH_STATUS_UNSPECIFIED": "UNKNOWN",
		"HEALTH_STATUS_HEALTHY":     "HEALTHY",
		"HEALTH_STATUS_UNHEALTHY":   "UNHEALTHY",
		"HEALTH_STATUS_DEGRADED":    "DEGRADED", // TrimPrefix fallback
		"totally-other":             "totally-other",
	}
	for in, want := range cases {
		if got := healthLabel(in); got != want {
			t.Errorf("healthLabel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHealthJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{map[string]any{"dashboardId": "d1"}}}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"d1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboards\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHealthEmptyDashboards(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{}}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestHealthRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestHealthRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestHealthMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &healthCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthWhitespacePositional(t *testing.T) {
	t.Parallel()
	// requireID also rejects a pure-whitespace positional so we don't POST
	// a meaningless request.
	cmd := &healthCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"   "}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestHealthBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"health", "d-1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHealthRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": "nope"}
			return nil
		},
	}
	cmd := &healthCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"d1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
