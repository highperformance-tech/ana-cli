package org

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestRolesListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.rbac.RBACService/ListRoles" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"roles": []any{
					map[string]any{"id": "r1", "name": "admin"},
					map[string]any{"id": "r2", "name": "member"},
				},
			}
			return nil
		},
	}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "r1", "admin", "r2", "member"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestRolesListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"roles": []any{}}
			return nil
		},
	}
	cmd := &rolesListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"roles\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestRolesListUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

// TestRolesListRejectsExtraPositionals pins the no-positional contract:
// trailing tokens after the verb path must yield ErrUsage before the RPC fires.
func TestRolesListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"roles", "list", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestRolesListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"roles", "list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestRolesListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"roles": "nope"}
			return nil
		},
	}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
