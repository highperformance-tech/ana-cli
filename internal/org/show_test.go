package org

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestShowTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"organization": map[string]any{
					"orgId":            "org-1",
					"organizationName": "Acme",
					"createdAt":        "2025-10-31T14:19:13Z",
				},
			}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"organizationName", "Acme", "orgId", "org-1", "createdAt", "2025-10-31"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Empty request body on the wire.
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestShowJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"organization": map[string]any{"orgId": "org-1"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"organization\"") {
		t.Errorf("stdout=%q want JSON", out.String())
	}
}

func TestShowUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("network boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "org show") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

// TestShowRejectsExtraPositionals pins the no-positional contract: trailing
// tokens after the verb path must yield ErrUsage before the RPC fires.
func TestShowRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"show", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestShowBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"show", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// organization is not an object — decoding into typed shape fails.
			*out = map[string]any{"organization": "not-an-object"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestShowJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"organization": map[string]any{"orgId": "x"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	err := cmd.Run(ctx, nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
