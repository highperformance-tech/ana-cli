package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/textql/ana-cli/internal/auth"
	"github.com/textql/ana-cli/internal/cli"
	"github.com/textql/ana-cli/internal/config"
	"github.com/textql/ana-cli/internal/transport"
)

// uuidRe matches canonical 8-4-4-4-12 lowercase-hex UUIDs. Used to sanity-check
// shape before we inspect the version/variant nibbles individually.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestNewUUID_Shape verifies the canonical v4 layout: 36 chars, 8-4-4-4-12,
// a literal '4' at byte 14 (version), and one of 8/9/a/b at byte 19 (variant).
func TestNewUUID_Shape(t *testing.T) {
	for i := 0; i < 64; i++ {
		u := newUUID()
		if len(u) != 36 {
			t.Fatalf("len=%d, want 36 (%q)", len(u), u)
		}
		if !uuidRe.MatchString(u) {
			t.Fatalf("uuid %q does not match canonical form", u)
		}
		if u[14] != '4' {
			t.Fatalf("version nibble = %q, want '4' in %q", u[14], u)
		}
		switch u[19] {
		case '8', '9', 'a', 'b':
			// ok
		default:
			t.Fatalf("variant nibble = %q, want one of 8/9/a/b in %q", u[19], u)
		}
	}
}

// TestNewUUID_Unique catches a trivial regression where the generator returns a
// constant. 64 iterations is overkill for collision avoidance but cheap.
func TestNewUUID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		u := newUUID()
		if _, dup := seen[u]; dup {
			t.Fatalf("duplicate uuid produced: %s", u)
		}
		seen[u] = struct{}{}
	}
}

// TestConfigTranslation covers the tiny projection helpers between the main
// package's config.Config and auth.Config. Trivial, but guards against future
// drift where a new field lands in one side and not the other.
func TestConfigTranslation(t *testing.T) {
	c := config.Config{Endpoint: "https://example.com", Token: "abc"}
	ac := toAuthConfig(c)
	if ac.Endpoint != c.Endpoint || ac.Token != c.Token {
		t.Fatalf("toAuthConfig lost fields: %+v", ac)
	}
	round := fromAuthConfig(ac)
	if round != c {
		t.Fatalf("round-trip mismatch: got %+v want %+v", round, c)
	}
}

// TestStreamAdapter_ReturnsSession calls streamAdapter against an httptest
// server that yields a single data frame + clean trailer, and verifies the
// returned value satisfies chat.StreamSession and delivers the frame.
func TestStreamAdapter_ReturnsSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(200)
		// Data frame: flags=0, len=9, payload {"x":1}
		payload := []byte(`{"x":1}`)
		hdr := []byte{0x00, 0x00, 0x00, 0x00, byte(len(payload))}
		w.Write(hdr)
		w.Write(payload)
		// Trailer frame: flags=0x02, len=0.
		w.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00})
	}))
	defer srv.Close()

	client := transport.New(srv.URL, func(context.Context) (string, error) { return "", nil })
	adapter := streamAdapter(client)
	sess, err := adapter(context.Background(), "/any", map[string]any{})
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	defer sess.Close()
	var got map[string]any
	ok, err := sess.Next(&got)
	if err != nil || !ok {
		t.Fatalf("Next: ok=%v err=%v", ok, err)
	}
	if got["x"].(float64) != 1 {
		t.Fatalf("payload: %+v", got)
	}
	ok, err = sess.Next(&got)
	if err != nil || ok {
		t.Fatalf("expected clean trailer; ok=%v err=%v", ok, err)
	}
}

// TestStreamAdapter_PropagatesError exercises the err != nil branch of the
// adapter: a 500 response with a Connect error envelope should surface as a
// transport error, and the session should be nil.
func TestStreamAdapter_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","message":"boom"}`))
	}))
	defer srv.Close()

	client := transport.New(srv.URL, func(context.Context) (string, error) { return "", nil })
	sess, err := streamAdapter(client)(context.Background(), "/any", nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if sess != nil {
		t.Fatalf("expected nil session on error, got %T", sess)
	}
}

// TestAuthDeps_LoadSave_RoundTrip drives authDeps against a real on-disk
// config file in t.TempDir(), exercising LoadCfg, SaveCfg, and ConfigPath in
// one test. This is the most compact way to cover the three closures.
func TestAuthDeps_LoadSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Seed with an existing file so LoadCfg returns non-zero.
	if err := config.Save(cfgPath, config.Config{Endpoint: "https://existing", Token: "t0"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	env := func(string) string { return "" }
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })

	deps := authDeps(client, env, cfgPath)

	got, err := deps.LoadCfg()
	if err != nil {
		t.Fatalf("LoadCfg: %v", err)
	}
	if got.Endpoint != "https://existing" || got.Token != "t0" {
		t.Fatalf("LoadCfg round-trip: %+v", got)
	}

	if err := deps.SaveCfg(auth.Config{Endpoint: "https://new", Token: "t1"}); err != nil {
		t.Fatalf("SaveCfg: %v", err)
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Endpoint != "https://new" || after.Token != "t1" {
		t.Fatalf("SaveCfg did not persist: %+v", after)
	}

	p, err := deps.ConfigPath()
	if err != nil || p != cfgPath {
		t.Fatalf("ConfigPath: p=%q err=%v", p, err)
	}
}

// TestAuthDeps_EmptyPath_FallsBackToEnv exercises the cfgPath=="" branches of
// SaveCfg/ConfigPath: with XDG_CONFIG_HOME set, both should resolve a path
// under that directory rather than erroring.
func TestAuthDeps_EmptyPath_FallsBackToEnv(t *testing.T) {
	dir := t.TempDir()
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	deps := authDeps(client, env, "")

	// LoadCfg with empty cfgPath returns zero value, no error.
	got, err := deps.LoadCfg()
	if err != nil || got != (auth.Config{}) {
		t.Fatalf("LoadCfg empty-path: got=%+v err=%v", got, err)
	}

	p, err := deps.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("ConfigPath did not honor XDG: %q", p)
	}

	if err := deps.SaveCfg(auth.Config{Endpoint: "https://e", Token: "t"}); err != nil {
		t.Fatalf("SaveCfg: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected config written at %q: %v", p, err)
	}
}

// TestBuildVerbs_Shape checks the top-level verb map registers every verb name
// promised by docs/features.md. Regression guard: a drop would be silent
// otherwise — main.go has no other assertion about verb naming.
func TestBuildVerbs_Shape(t *testing.T) {
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	verbs := buildVerbs(client, func(string) string { return "" }, "")
	want := []string{"auth", "org", "connector", "chat", "dashboard", "playbook", "ontology", "feed", "audit"}
	for _, v := range want {
		if _, ok := verbs[v]; !ok {
			t.Errorf("missing verb: %q", v)
		}
	}
	if len(verbs) != len(want) {
		t.Errorf("verb count = %d, want %d (verbs=%v)", len(verbs), len(want), verbs)
	}
}

// TestChatDeps_Fields smoke-tests chatDeps wiring. We don't invoke Stream here
// (that's covered by TestStreamAdapter_*), just assert the deps struct is
// populated.
func TestChatDeps_Fields(t *testing.T) {
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	d := chatDeps(client)
	if d.Unary == nil || d.Stream == nil || d.UUIDFn == nil {
		t.Fatalf("chatDeps missing fields: %+v", d)
	}
	if u := d.UUIDFn(); !uuidRe.MatchString(u) {
		t.Fatalf("UUIDFn returned non-canonical: %q", u)
	}
}

// TestRun_NoArgs_PrintsHelp verifies the top-level behavior: no args prints
// root help to stdout and returns ErrUsage (exit code 1 via cli.ExitCode).
func TestRun_NoArgs_PrintsHelp(t *testing.T) {
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	err := run(nil, stdio, func(string) string { return "" })
	if cli.ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", cli.ExitCode(err), err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected help on stdout, got: %q", out.String())
	}
}

// TestRun_EndToEnd_ConnectorList is the v1 e2e smoke test: spin up an
// httptest.Server returning a canned ConnectorService.List response, point
// --endpoint at it, run `connector list --json`, and assert the response
// landed on stdout. This exercises ParseGlobal -> transport.New -> verb
// dispatch all the way through.
func TestRun_EndToEnd_ConnectorList(t *testing.T) {
	// Connector list returns {"connectors": [...]}. We capture the request
	// path so the test also verifies we built the right URL.
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{
			"connectors": []map[string]any{{"id": 1, "name": "alpha"}},
		})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	// --endpoint overrides everything; empty token is fine for this server.
	args := []string{"--endpoint", srv.URL, "--json", "connector", "list"}
	err := run(args, stdio, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, errb.String())
	}
	if !strings.Contains(gotPath, "ConnectorService") {
		t.Fatalf("expected request to ConnectorService path, got %q", gotPath)
	}
	if !strings.Contains(out.String(), "alpha") {
		t.Fatalf("expected 'alpha' in stdout, got: %q", out.String())
	}
}
