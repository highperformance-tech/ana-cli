package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- service-accounts list ---

func TestSAListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"serviceAccounts": []any{
					map[string]any{"memberId": "m1", "displayName": "Name", "description": "D"},
					map[string]any{"memberId": "m2", "displayName": "Other", "email": "e@x"},
				},
			}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "DESCRIPTION") {
		t.Errorf("headers: %q", s)
	}
	// Description fall-through to email when blank.
	if !strings.Contains(s, "e@x") {
		t.Errorf("fallback email missing: %q", s)
	}
}

func TestSAListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"serviceAccounts": []any{}}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"serviceAccounts\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestSAListRejectsExtraPositionals pins the no-positional contract: trailing
// tokens after the verb path must yield ErrUsage before the RPC fires.
func TestSAListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "list", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestSAListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &saListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSAListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"serviceAccounts": "nope"}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestSAListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- service-accounts create ---

func TestSACreateHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createServiceAccountResp)
			out.MemberID = "m1"
			out.Name = "Name"
			return nil
		},
	}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--name", "probe", "--description", "d"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "m1") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"name":"probe"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if !strings.Contains(string(f.lastRawReq), `"description":"d"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSACreateNoRespName(t *testing.T) {
	t.Parallel()
	// Response leaves Name empty; we should echo the request-provided name.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createServiceAccountResp)
			out.MemberID = "m1"
			return nil
		},
	}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--name", "probe"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "probe") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSACreateMissingName(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestSACreateEmptyName(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--name", ""}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "empty") {
		t.Errorf("err=%v", err)
	}
}

// TestSACreateRejectsExtraPositionals pins the no-positional contract for
// `auth service-accounts create`: any trailing token must yield ErrUsage
// before RequireFlags or any RPC fires.
func TestSACreateRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--name", "n", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestSACreateUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--name", "n"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSACreateBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "create", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- service-accounts delete ---

func TestSADeleteHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"m1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted m1") {
		t.Errorf("stdout=%q", out.String())
	}
	// memberId (not serviceAccountId) per catalog.
	if !strings.Contains(string(f.lastRawReq), `"memberId":"m1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSADeleteMissingPositional(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSADeleteUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSADeleteBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"service-accounts", "delete", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSADeleteExtraPositional(t *testing.T) {
	t.Parallel()
	called := 0
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { called++; return nil }}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if called != 0 {
		t.Errorf("Unary must not be invoked on arity failure, called=%d", called)
	}
}
