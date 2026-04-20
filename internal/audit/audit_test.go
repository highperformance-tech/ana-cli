package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- fakes and helpers ---

type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	now        time.Time
	lastPath   string
	lastReq    any
	lastRawReq []byte
}

// deps returns a Deps whose Unary records every call and whose Now returns
// f.now so --since produces a deterministic request body.
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
		Now: func() time.Time { return f.now },
	}
}

// --- New / Group surface ---

func TestNewReturnsGroupWithTailChild(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	g := New(f.deps())
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	if _, ok := g.Children["tail"]; !ok {
		t.Errorf("missing tail child")
	}
}

// A zero Deps.Now should be defaulted to time.Now by New.
func TestNewDefaultsNow(t *testing.T) {
	t.Parallel()
	g := New(Deps{Unary: func(context.Context, string, any, any) error { return nil }})
	if g == nil {
		t.Fatalf("New returned nil")
	}
	// Fish out the tail command and check its Deps.Now is non-nil.
	tc, ok := g.Children["tail"].(*tailCmd)
	if !ok {
		t.Fatalf("tail child not *tailCmd")
	}
	if tc.deps.Now == nil {
		t.Errorf("Now not defaulted")
	}
}

// --- Help() ---

func TestHelpNonEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	h := cmd.Help()
	if h == "" || !strings.Contains(strings.ToLower(h), "usage") {
		t.Errorf("help=%q", h)
	}
}
