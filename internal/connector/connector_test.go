package connector

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
// (camelCase field names, omitted-when-empty behavior, etc.).
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

// errReader trips the password-stdin / scanner error path.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	t.Parallel()
	g := New(Deps{})
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	expected := []string{"list", "get", "create", "update", "delete", "test", "tables", "examples"}
	for _, name := range expected {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
}

// --- Help() coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	cases := map[string]cli.Command{
		"list":     &listCmd{},
		"get":      &getCmd{},
		"create":   &createCmd{},
		"update":   &updateCmd{},
		"delete":   &deleteCmd{},
		"test":     &testCmd{},
		"tables":   &tablesCmd{},
		"examples": &examplesCmd{},
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
