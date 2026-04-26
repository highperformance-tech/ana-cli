package connector

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
				"connectors": []any{
					map[string]any{"id": 1.0, "name": "alpha", "connectorType": "POSTGRES"},
					map[string]any{"id": 2.0, "name": "beta", "connectorType": "BIGQUERY"},
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
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "TYPE") {
		t.Errorf("missing headers: %q", s)
	}
	if !strings.Contains(s, "alpha") || !strings.Contains(s, "beta") {
		t.Errorf("missing rows: %q", s)
	}
	if f.lastPath != servicePath+"/GetConnectors" {
		t.Errorf("path=%s", f.lastPath)
	}
	// Request is empty object.
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectors": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectors\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
	// And wrapped context.
	if err == nil || !strings.Contains(err.Error(), "connector list") {
		t.Errorf("err=%v", err)
	}
}

// TestListRejectsExtraPositionals pins the no-positional contract: trailing
// tokens after the verb path must yield ErrUsage before the RPC fires.
func TestListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
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
			*out = map[string]any{"connectors": "not-an-array"}
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
