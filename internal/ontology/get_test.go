package ontology

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
			if path != ontologyServicePath+"/GetOntologyById" {
				t.Errorf("path=%s", path)
			}
			b, _ := json.Marshal(req)
			if string(b) != `{"ontologyId":5476}` {
				t.Errorf("req=%s", string(b))
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"ontology": map[string]any{
					"id":          5476,
					"name":        "TextQL Usage Ontology",
					"description": "Auto-generated ontology for TextQL Usage connector",
					"connectorId": 3847,
				},
			}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"5476"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{
		"id", "5476", "name", "TextQL Usage Ontology",
		"description", "Auto-generated ontology",
		"connectorId", "3847",
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
			*out = map[string]any{"ontology": map[string]any{"id": 5476}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"5476"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q should start with {", out.String())
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

func TestGetNonIntID(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
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
	err := cmd.Run(context.Background(), []string{"5476"}, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestGetBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestGetRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"ontology": "not-an-object"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"5476"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestGetEmptyOntologyFallsBackToJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": "x"}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"5476"}, stdio); err != nil {
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
			*out = map[string]any{"ontology": map[string]any{"id": 5476}}
			return nil
		},
	}
	cmd := &getCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, []string{"5476"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

func TestGetEmptyOntologyFallbackJSONEncodeErr(t *testing.T) {
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
	if err := cmd.Run(context.Background(), []string{"5476"}, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
