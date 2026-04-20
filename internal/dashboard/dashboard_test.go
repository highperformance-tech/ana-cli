package dashboard

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps captures each Unary invocation's path + encoded request bytes so
// tests can assert on both the endpoint and the wire-level JSON shape
// (camelCase field names, array-vs-scalar, etc.).
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	lastPath   string
	lastRawReq []byte
}

func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
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
	g := New(Deps{})
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	for _, name := range []string{"list", "folders", "get", "spawn", "health"} {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	// folders is itself a group with a list child.
	folders, ok := g.Children["folders"].(*cli.Group)
	if !ok {
		t.Fatalf("folders child must be *cli.Group")
	}
	if _, ok := folders.Children["list"]; !ok {
		t.Errorf("folders group missing list child")
	}
	if folders.Summary == "" {
		t.Errorf("folders Summary should be non-empty")
	}
}

func TestHelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	cases := map[string]cli.Command{
		"list":         &listCmd{},
		"folders-list": &foldersListCmd{},
		"get":          &getCmd{},
		"spawn":        &spawnCmd{},
		"health":       &healthCmd{},
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
