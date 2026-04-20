package org

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
