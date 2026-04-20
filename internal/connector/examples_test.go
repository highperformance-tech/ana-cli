package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestExamplesHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"examples": []any{
					map[string]any{"label": "Explore", "message": "hi", "category": "Exploration"},
				},
			}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"608"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "[Exploration] Explore: hi") {
		t.Errorf("stdout=%q", out.String())
	}
	// Body wraps id in connectorContexts array.
	if !strings.Contains(string(f.lastRawReq), `"connectorContexts":[{"connectorId":608}]`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if f.lastPath != servicePath+"/GetExampleQueries" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestExamplesJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"examples": []any{}}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"examples\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestExamplesMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesNonInt(t *testing.T) {
	t.Parallel()
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &examplesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestExamplesRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"examples": "nope"}
			return nil
		},
	}
	cmd := &examplesCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
