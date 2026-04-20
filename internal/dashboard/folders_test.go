package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestFoldersListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"folders": []any{
					map[string]any{"id": "b", "name": "Beta"},
					map[string]any{"id": "a", "name": "Alpha"},
				},
			}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	// Alpha must appear before Beta thanks to the sort.
	if ai, bi := strings.Index(s, "Alpha"), strings.Index(s, "Beta"); ai < 0 || bi < 0 || ai > bi {
		t.Errorf("sort broken: %q", s)
	}
	if f.lastPath != servicePath+"/ListDashboardFolders" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestFoldersListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"folders": []any{}}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"folders\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestFoldersListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestFoldersListBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &foldersListCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestFoldersListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"folders": "nope"}
			return nil
		},
	}
	cmd := &foldersListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
