package org

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps is the table-driven fake for Deps. The unary function field
// defaults to a benign no-op; individual tests override what they need. Each
// call's path and JSON-encoded request are recorded so assertions can inspect
// the wire-level payload the command produced.
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

// deps returns a Deps whose Unary funnels through the fake so tests can
// assert on recorded inputs after the command runs.
func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			f.lastReq = req
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
	}
}

// newIO builds a cli.IO with in-memory streams so tests can assert on output
// without touching the real file descriptors.
func newIO() (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

// failingWriter returns err on every Write so we can trip json.Encoder paths.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("w boom") }

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	f := &fakeDeps{}
	g := New(f.deps())
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	expected := []string{"show", "list", "members", "roles", "permissions"}
	for _, name := range expected {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	// members, roles, permissions must themselves be groups with a `list` child.
	for _, n := range []string{"members", "roles", "permissions"} {
		sub, ok := g.Children[n].(*cli.Group)
		if !ok {
			t.Errorf("%s should be a *cli.Group", n)
			continue
		}
		if _, ok := sub.Children["list"]; !ok {
			t.Errorf("%s group missing `list` child", n)
		}
		if sub.Summary == "" {
			t.Errorf("%s group missing Summary", n)
		}
	}
}

// --- Help() text coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"show":        &showCmd{deps: f.deps()},
		"list":        &listCmd{deps: f.deps()},
		"membersList": &membersListCmd{deps: f.deps()},
		"rolesList":   &rolesListCmd{deps: f.deps()},
		"permsList":   &permissionsListCmd{deps: f.deps()},
	}
	for n, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", n)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", n, h)
		}
	}
}

// --- show ---

func TestShowTable(t *testing.T) {
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
	stdio, out, _ := newIO()
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
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"organization": map[string]any{"orgId": "org-1"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"organization\"") {
		t.Errorf("stdout=%q want JSON", out.String())
	}
}

func TestShowUnaryErr(t *testing.T) {
	boom := errors.New("network boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "org show") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestShowBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// organization is not an object — decoding into typed shape fails.
			*out = map[string]any{"organization": "not-an-object"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestShowJSONEncodeErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"organization": map[string]any{"orgId": "x"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	err := cmd.Run(ctx, nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// --- list ---

func TestListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != "/rpc/public/textql.rpc.public.auth.PublicAuthService/ListOrganizations" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			// Server order is intentionally non-alphabetical so the sort is observable.
			*out = map[string]any{
				"organizations": []any{
					map[string]any{
						"orgId":              "org-z",
						"organizationName":   "Zeta",
						"defaultConnectorId": float64(42),
					},
					map[string]any{
						"orgId":            "org-a",
						"organizationName": "acme",
					},
					map[string]any{
						"orgId":            "org-m",
						"organizationName": "Midway",
					},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"NAME", "ORG ID", "DEFAULT CONNECTOR", "Zeta", "acme", "Midway", "org-a", "org-m", "org-z", "42"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Case-insensitive sort: acme < Midway < Zeta.
	if ai, mi, zi := strings.Index(s, "acme"), strings.Index(s, "Midway"), strings.Index(s, "Zeta"); !(ai < mi && mi < zi) {
		t.Errorf("sort order wrong: acme=%d midway=%d zeta=%d in %q", ai, mi, zi, s)
	}
	// Row for an org without defaultConnectorId renders an empty cell — verify
	// the line for Midway ends without a trailing number.
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "Midway") {
			trimmed := strings.TrimRight(line, " ")
			if strings.HasSuffix(trimmed, "42") || strings.HasSuffix(trimmed, "0") {
				t.Errorf("Midway row should have empty connector cell, got %q", line)
			}
		}
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// Include a field the table doesn't render to verify raw passthrough.
			*out = map[string]any{
				"organizations": []any{
					map[string]any{
						"orgId":            "org-1",
						"organizationName": "Acme",
						"theme":            map[string]any{"bg": "#fff"},
					},
				},
			}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"\"organizations\"", "\"theme\"", "\"bg\"", "#fff"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in JSON output %q", want, s)
		}
	}
}

func TestListUnaryErr(t *testing.T) {
	boom := errors.New("network boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "org list") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// organizations is a string — decoding into []struct fails.
			*out = map[string]any{"organizations": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestListHelp(t *testing.T) {
	f := &fakeDeps{}
	cmd := &listCmd{deps: f.deps()}
	h := cmd.Help()
	for _, want := range []string{"list", "ana org list"} {
		if !strings.Contains(h, want) {
			t.Errorf("help missing %q: %q", want, h)
		}
	}
}

// --- members list ---

func TestMembersListTable(t *testing.T) {
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
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "EMAIL", "ROLE", "m1", "a@b.c", "member", "m2", "x@y.z", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	if got := string(f.lastRawReq); got != `{"orgId":"org-1"}` {
		t.Errorf("req=%s want {\"orgId\":\"org-1\"}", got)
	}
}

func TestMembersListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path == "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization" {
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"members": []any{}}
			return nil
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"members\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestMembersListUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
}

func TestMembersListCallErr(t *testing.T) {
	// GetOrganization succeeds; ListOrganizationMembers itself errors.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path == "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization" {
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			}
			return errors.New("list-boom")
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "list-boom") {
		t.Errorf("err=%v", err)
	}
}

func TestMembersListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestMembersListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path == "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetOrganization" {
				out := resp.(*struct {
					Organization struct {
						OrgID string `json:"orgId"`
					} `json:"organization"`
				})
				out.Organization.OrgID = "org-1"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"members": "not-an-array"}
			return nil
		},
	}
	cmd := &membersListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- roles list ---

func TestRolesListTable(t *testing.T) {
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
	stdio, out, _ := newIO()
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
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"roles": []any{}}
			return nil
		},
	}
	cmd := &rolesListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"roles\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestRolesListUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestRolesListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestRolesListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"roles": "nope"}
			return nil
		},
	}
	cmd := &rolesListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- permissions list ---

func TestPermissionsListTable(t *testing.T) {
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
	stdio, out, _ := newIO()
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "NAME", "p1", "api_access_key:read", "p2", "chat", "p3", "write", "p4", "-"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
}

func TestPermissionsListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"permissions": []any{}}
			return nil
		},
	}
	cmd := &permissionsListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO()
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"permissions\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestPermissionsListUnaryErr(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestPermissionsListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestPermissionsListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"permissions": "nope"}
			return nil
		},
	}
	cmd := &permissionsListCmd{deps: f.deps()}
	stdio, _, _ := newIO()
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
