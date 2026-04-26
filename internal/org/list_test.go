package org

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
	stdio, out, _ := testcli.NewIO(nil)
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
	t.Parallel()
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
	stdio, out, _ := testcli.NewIO(nil)
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
	t.Parallel()
	boom := errors.New("network boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "org list") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestListBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"list", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			// organizations is a string — decoding into []struct fails.
			*out = map[string]any{"organizations": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestListHelp(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &listCmd{deps: f.deps()}
	h := cmd.Help()
	for _, want := range []string{"list", "ana org list"} {
		if !strings.Contains(h, want) {
			t.Errorf("help missing %q: %q", want, h)
		}
	}
}
