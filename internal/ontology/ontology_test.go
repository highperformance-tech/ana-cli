package ontology

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps records the path and JSON-encoded request body each call produced.
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

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
	for _, name := range []string{"list", "get"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
}

// --- Help() text coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"list": &listCmd{deps: f.deps()},
		"get":  &getCmd{deps: f.deps()},
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
