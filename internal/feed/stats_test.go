package feed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestStatsTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != feedServicePath+"/GetFeedStats" {
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
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	// Row-coupled assertions: a bare `strings.Contains(s, "3")` would false-pass
	// if the "3" appeared on some unrelated row (e.g. part of "399"). Pin each
	// label+value pair to the same rendered line so cross-row bleed cannot mask
	// a regression.
	s := out.String()
	lines := strings.Split(strings.TrimSpace(s), "\n")
	rowHas := func(label, value string) bool {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, label) && strings.Contains(trimmed, value) {
				return true
			}
		}
		return false
	}
	for label, value := range map[string]string{
		"messagesToday":        "116",
		"messagesAllTime":      "11518",
		"activeAgents":         "3",
		"dashboardsCreated":    "7",
		"threadsCreated":       "399",
		"playbooksCreated":     "5",
		"connectorsConfigured": "4",
		"connectorNames":       "HPT, tableau.example.com",
		"activeAgentNames":     "AWS Inspector",
	} {
		if !rowHas(label, value) {
			t.Errorf("expected %s=%q on same line in output %q", label, value, s)
		}
	}
}

func TestStatsTableEmptyLists(t *testing.T) {
	t.Parallel()
	// Empty lists should render as "-" so tabwriter keeps alignment.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	// Row-specific dash assertion: bare `strings.Contains(s, "-")` is
	// unsafe because other rendered values could accidentally carry a
	// hyphen. Pin each empty-list row to end with "-" so the empty-list
	// branch of joinOrDash is genuinely covered for every slice field.
	s := out.String()
	lines := strings.Split(strings.TrimSpace(s), "\n")
	rowEndsWithDash := func(label string) bool {
		for _, line := range lines {
			if strings.Contains(line, label) && strings.HasSuffix(strings.TrimSpace(line), "-") {
				return true
			}
		}
		return false
	}
	for _, label := range []string{"connectorNames", "activeAgentNames"} {
		if !rowEndsWithDash(label) {
			t.Errorf("expected %s row to end with '-' for the empty-list branch: %q", label, s)
		}
	}
}

func TestStatsJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": 1}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestStatsUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestStatsBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestStatsRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": "not-a-number"}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestStatsJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"messagesToday": 1}
			return nil
		},
	}
	cmd := &statsCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

func TestJoinOrDash(t *testing.T) {
	t.Parallel()
	if got := joinOrDash(nil); got != "-" {
		t.Errorf("nil -> %q", got)
	}
	if got := joinOrDash([]string{"a", "b"}); got != "a, b" {
		t.Errorf("got %q", got)
	}
}
