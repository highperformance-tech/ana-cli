package org

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestMembersListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			switch path {
			case "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization":
				// Decode into the pre-call's typed struct. Use a blind
				// assignment through reflection-free path by writing via a
				// JSON round-trip.
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			case "/rpc/public/textql.rpc.public.settings.SettingsService/ListOrganizationMembers":
				out := resp.(*map[string]any)
				*out = map[string]any{
					"members": []any{
						map[string]any{"memberId": "m1", "emailAddress": "a@b.c", "role": "member"},
						map[string]any{"memberId": "m2", "emailAddress": "x@y.z"},
					},
				}
				return nil
			default:
				t.Errorf("path=%s", path)
				return nil
			}
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "EMAIL", "ROLE", "m1", "a@b.c", "member", "m2", "x@y.z"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: m2 has no role, so its trailing ROLE
	// cell must render as "-". Bare substring checks are too loose.
	foundM2Dash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.Contains(line, "m2") && strings.HasSuffix(strings.TrimSpace(line), "-") {
			foundM2Dash = true
			break
		}
	}
	if !foundM2Dash {
		t.Errorf("expected m2 row to end with '-' ROLE placeholder: %q", s)
	}
	if got := string(f.lastRawReq); got != `{"orgId":"org-1"}` {
		t.Errorf("req=%s want {\"orgId\":\"org-1\"}", got)
	}
}

func TestMembersListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			switch path {
			case "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization":
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			case "/rpc/public/textql.rpc.public.settings.SettingsService/ListOrganizationMembers":
				out := resp.(*map[string]any)
				*out = map[string]any{"members": []any{}}
				return nil
			default:
				t.Errorf("unexpected path=%s", path)
				return nil
			}
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"members\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestMembersListUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestMembersListCallErr(t *testing.T) {
	t.Parallel()
	// GetOrganization succeeds; ListOrganizationMembers itself errors.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			switch path {
			case "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization":
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			case "/rpc/public/textql.rpc.public.settings.SettingsService/ListOrganizationMembers":
				return errors.New("list-boom")
			default:
				t.Errorf("unexpected path=%s", path)
				return nil
			}
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "list-boom") {
		t.Errorf("err=%v", err)
	}
}

// TestMembersListRejectsExtraPositionals pins the no-positional contract:
// trailing tokens after the verb path must yield ErrUsage before the RPC fires.
func TestMembersListRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"members", "list", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestMembersListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"members", "list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestMembersListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			switch path {
			case "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization":
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			case "/rpc/public/textql.rpc.public.settings.SettingsService/ListOrganizationMembers":
				out := resp.(*map[string]any)
				*out = map[string]any{"members": "not-an-array"}
				return nil
			default:
				t.Errorf("unexpected path=%s", path)
				return nil
			}
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
