package ontology

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != ontologyServicePath+"/GetOntologies" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"ontologies": []any{
					map[string]any{"id": int64(5476), "name": "TextQL Usage Ontology"},
					map[string]any{"id": int64(2200), "name": "HPT Ontology"},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "5476", "TextQL Usage Ontology", "2200", "HPT Ontology"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"ontologies": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q should start with {", out.String())
	}
	if !strings.Contains(out.String(), "\"ontologies\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("net boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "ontology list") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

// TestListRejectsExtraPositionals pins the no-positional contract: trailing
// tokens after the verb path must yield ErrUsage before the RPC fires.
func TestListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"list", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"ontologies": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestListJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"ontologies": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
