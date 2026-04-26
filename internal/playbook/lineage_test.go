package playbook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestLineageEmptyResponse(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != playbookServicePath+"/GetPlaybookLineage" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "no lineage edges") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageEdgesTable(t *testing.T) {
	t.Parallel()
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
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"FROM", "TO", "TYPE", "a", "b", "depends_on", "c", "d"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: the empty-map edge fixture has no
	// from/to/source/target/type, so every cell in its row must render as
	// "-". A bare substring check would be ambiguous with the c/d edge
	// row, whose TYPE cell also renders as "-". Assert the all-dashes row
	// exists explicitly (three "-" fields).
	foundAllDashRow := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "-" && fields[1] == "-" && fields[2] == "-" {
			foundAllDashRow = true
			break
		}
	}
	if !foundAllDashRow {
		t.Errorf("expected fully-empty edge to render as a row of three '-' cells: %q", s)
	}
}

func TestLineageLineageFallback(t *testing.T) {
	t.Parallel()
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
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "x") || !strings.Contains(out.String(), "y") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageNodesFallback(t *testing.T) {
	t.Parallel()
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
	stdio, out, _ := testcli.NewIO(nil)
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
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": []any{}}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"edges\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLineageMissingID(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestLineageUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestLineageBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"lineage", "p1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLineageRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": "nope"}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"pb1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestLineageJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"edges": []any{}}
			return nil
		},
	}
	cmd := &lineageCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, []string{"pb1"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
