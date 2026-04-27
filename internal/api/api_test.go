package api

import (
	"context"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps is the package-wide fake for Deps. DoRaw delegates to doRawFn if
// set; either way it records (method, path, body) for post-call assertions.
type fakeDeps struct {
	doRawFn    func(ctx context.Context, method, path string, body []byte) (int, []byte, error)
	lastMethod string
	lastPath   string
	lastBody   []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		DoRaw: func(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
			f.lastMethod = method
			f.lastPath = path
			// Copy so callers that reuse the slice don't mutate recorded state.
			if body != nil {
				f.lastBody = append(f.lastBody[:0], body...)
			} else {
				f.lastBody = nil
			}
			if f.doRawFn != nil {
				return f.doRawFn(ctx, method, path, body)
			}
			return 200, []byte(`{"ok":true}`), nil
		},
	}
}

// runAPI wraps the api leaf in a minimal Group so the resolve-then-parse
// pipeline runs (flag tokens get parsed, positionals get separated). The api
// package's New returns a leaf rather than a *cli.Group, so tests that want
// flag parsing must dispatch through a wrapper Group.
func runAPI(ctx context.Context, deps Deps, args []string, stdio cli.IO) error {
	g := &cli.Group{Children: map[string]cli.Command{"api": New(deps)}}
	return g.Run(ctx, append([]string{"api"}, args...), stdio)
}

// --- New / leaf surface ---

func TestNewReturnsLeaf(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	if cmd == nil {
		t.Fatalf("New returned nil")
	}
	// Must NOT be a *cli.Group — api is a single-leaf verb.
	if _, isGroup := cmd.(*cli.Group); isGroup {
		t.Fatalf("api.New returned *cli.Group; want a leaf Command")
	}
	if _, isFlagger := cmd.(cli.Flagger); !isFlagger {
		t.Fatalf("api leaf should implement cli.Flagger so --help can render a Flags block")
	}
}

// --- Help() ---

func TestHelpContainsBothPathForms(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	h := cmd.Help()
	for _, want := range []string{
		"Usage",
		"<service>/<Method>",
		"/rpc/public/",
		"/v1/",
		"--raw",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("help missing %q in:\n%s", want, h)
		}
	}
}
