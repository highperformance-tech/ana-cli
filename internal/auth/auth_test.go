package auth

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps is the table-driven fake for Deps. Each function field defaults to
// a benign implementation; individual tests override what they need. Every
// call is recorded so assertions can inspect the request payload that flowed
// through Unary. The mu guards the recorded fields because whoami (and any
// future fan-out command) invokes Unary concurrently from goroutines — without
// the lock the race detector flags simultaneous writes to lastPath etc.
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	loadFn     func() (Config, error)
	saveFn     func(Config) error
	pathFn     func() (string, error)
	mu         sync.Mutex
	lastPath   string
	lastReq    any
	lastRawReq []byte
	saved      *Config
	saveCalls  int
	loadCalls  int
	pathCalls  int
}

// deps returns a Deps whose functions funnel through the fake so tests can
// assert on recorded inputs after the command runs.
func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.mu.Lock()
			f.lastPath = path
			f.lastReq = req
			// Capture the JSON-encoded request so tests can assert on exact
			// wire-level field names (camelCase check).
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			f.mu.Unlock()
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
		LoadCfg: func() (Config, error) {
			f.loadCalls++
			if f.loadFn != nil {
				return f.loadFn()
			}
			return Config{}, nil
		},
		SaveCfg: func(c Config) error {
			f.saveCalls++
			f.saved = &c
			if f.saveFn != nil {
				return f.saveFn(c)
			}
			return nil
		},
		ConfigPath: func() (string, error) {
			f.pathCalls++
			if f.pathFn != nil {
				return f.pathFn()
			}
			return "/tmp/ana/config.json", nil
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
	expected := []string{"login", "logout", "whoami", "keys", "service-accounts"}
	for _, name := range expected {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	// keys and service-accounts must themselves be groups with children.
	if kg, ok := g.Children["keys"].(*cli.Group); !ok || len(kg.Children) != 4 {
		t.Errorf("keys should be a group with 4 children")
	}
	if sg, ok := g.Children["service-accounts"].(*cli.Group); !ok || len(sg.Children) != 3 {
		t.Errorf("service-accounts should be a group with 3 children")
	}
}

// --- Help() text coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"login":   &loginCmd{deps: f.deps()},
		"logout":  &logoutCmd{deps: f.deps()},
		"whoami":  &whoamiCmd{deps: f.deps()},
		"list":    &keysListCmd{deps: f.deps()},
		"create":  &keysCreateCmd{deps: f.deps()},
		"rotate":  &keysRotateCmd{deps: f.deps()},
		"revoke":  &keysRevokeCmd{deps: f.deps()},
		"saList":  &saListCmd{deps: f.deps()},
		"saCreat": &saCreateCmd{deps: f.deps()},
		"saDel":   &saDeleteCmd{deps: f.deps()},
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
