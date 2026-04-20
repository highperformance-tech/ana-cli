package feed

import (
	"context"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- fakes and helpers ---

type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{Unary: testcli.RecordUnary(&f.lastPath, &f.lastRawReq,
		func(ctx context.Context, path string, req, resp any) error {
			f.lastReq = req
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		})}
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
	for _, name := range []string{"show", "stats"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
}

// --- Help() coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"show":  &showCmd{deps: f.deps()},
		"stats": &statsCmd{deps: f.deps()},
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
