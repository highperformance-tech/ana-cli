package org

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestPermissionsListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.rbac.RBACService/ListPermissions" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"permissions": []any{
					map[string]any{"id": "p1", "resource": "api_access_key", "action": "read"},
					map[string]any{"id": "p2", "resource": "chat"},
					map[string]any{"id": "p3", "action": "write"},
					map[string]any{"id": "p4"},
				},
			}
			return nil
		},
	}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "p1", "api_access_key:read", "p2", "chat", "p3", "write", "p4"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: p4 has neither resource nor action, so
	// permissionName() falls back to "-". Pin that precise rendering (the
	// p4 row's NAME cell is "-") rather than a bare substring check, which
	// could pass for unrelated reasons.
	foundP4Dash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.Contains(line, "p4") && strings.HasSuffix(strings.TrimSpace(line), "-") {
			foundP4Dash = true
			break
		}
	}
	if !foundP4Dash {
		t.Errorf("expected p4 row to end with '-' NAME placeholder: %q", s)
	}
}

func TestPermissionsListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"permissions": []any{}}
			return nil
		},
	}
	cmd := &permissionsListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"permissions\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestPermissionsListUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestPermissionsListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"permissions", "list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestPermissionsListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"permissions": "nope"}
			return nil
		},
	}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
