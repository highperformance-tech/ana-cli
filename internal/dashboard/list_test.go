package dashboard

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
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"dashboards": []any{
					map[string]any{"id": "d1", "name": "alpha", "folderName": "F1"},
					map[string]any{"id": "d2", "name": "beta", "folderId": "fid-2"},
					map[string]any{"id": "d3", "name": "gamma"},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "FOLDER", "alpha", "beta", "gamma", "F1", "fid-2"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %q", want, s)
		}
	}
	// Row-specific dash assertion: the fid-2 cell already contains a "-",
	// so a bare `strings.Contains(s, "-")` would pass even if the empty
	// FOLDER cell were rendered wrong. The gamma row (d3 fixture) has no
	// folder fields, so that row's trailing FOLDER cell must render as "-".
	foundGammaDash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.Contains(line, "gamma") && strings.HasSuffix(strings.TrimSpace(line), "-") {
			foundGammaDash = true
			break
		}
	}
	if !foundGammaDash {
		t.Errorf("expected gamma row to end with '-' FOLDER placeholder: %q", s)
	}
	if f.lastPath != servicePath+"/ListDashboards" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"dashboards\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "dashboard list") {
		t.Errorf("err=%v", err)
	}
}

func TestListBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"dashboards": "nope"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
