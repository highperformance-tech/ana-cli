package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/auth"
	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
	"github.com/highperformance-tech/ana-cli/internal/transport"
)

// uuidRe matches canonical 8-4-4-4-12 lowercase-hex UUIDs. Used to sanity-check
// shape before we inspect the version/variant nibbles individually.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestNewUUID_Shape verifies the canonical v4 layout: 36 chars, 8-4-4-4-12,
// a literal '4' at byte 14 (version), and one of 8/9/a/b at byte 19 (variant).
func TestNewUUID_Shape(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	seen := make(map[string]struct{}, 64)
	for i := 0; i < 64; i++ {
		u := newUUID()
		if _, dup := seen[u]; dup {
			t.Fatalf("duplicate uuid produced: %s", u)
		}
		seen[u] = struct{}{}
	}
}

// TestProfileToAuthConfig covers the projection helper between config.Profile
// and auth.Config. Trivial, but guards against future drift where a new field
// lands in one side and not the other.
func TestProfileToAuthConfig(t *testing.T) {
	t.Parallel()
	p := config.Profile{Endpoint: "https://example.com", Token: "abc", OrgName: "Acme"}
	ac := profileToAuthConfig(p)
	if ac.Endpoint != p.Endpoint || ac.Token != p.Token {
		t.Fatalf("profileToAuthConfig lost fields: %+v", ac)
	}
}

// TestStreamAdapter_ReturnsSession calls streamAdapter against an httptest
// server that yields a single data frame + clean trailer, and verifies the
// returned value satisfies chat.StreamSession and delivers the frame.
func TestStreamAdapter_ReturnsSession(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// Seed the "default" profile so LoadCfg has something to read. Also
	// include OrgName to verify SaveCfg preserves it.
	seed := config.Config{
		Profiles: map[string]config.Profile{
			"default": {Endpoint: "https://existing", Token: "t0", OrgName: "Existing Co"},
		},
		Active: "default",
	}
	if err := config.Save(cfgPath, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	env := func(string) string { return "" }
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })

	deps := authDeps(client, env, cfgPath, "default")

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
	p, ok := after.Profiles["default"]
	if !ok {
		t.Fatalf("default profile missing after save: %+v", after)
	}
	if p.Endpoint != "https://new" || p.Token != "t1" {
		t.Fatalf("SaveCfg did not persist: %+v", p)
	}
	if p.OrgName != "Existing Co" {
		t.Errorf("OrgName not preserved: %q", p.OrgName)
	}

	pp, err := deps.ConfigPath()
	if err != nil || pp != cfgPath {
		t.Fatalf("ConfigPath: p=%q err=%v", pp, err)
	}
}

// TestAuthDeps_SaveCfg_DefaultsToNamedSlot covers the fallback: when run
// hands buildVerbs an empty profileName (shouldn't happen in practice, but
// authDeps defends against it), SaveCfg must still write into "default".
func TestAuthDeps_SaveCfg_DefaultsToNamedSlot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	env := func(string) string { return "" }
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	deps := authDeps(client, env, cfgPath, "")
	if err := deps.SaveCfg(auth.Config{Endpoint: "https://new", Token: "t1"}); err != nil {
		t.Fatalf("SaveCfg: %v", err)
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := after.Profiles["default"]; !ok {
		t.Fatalf("expected default slot, got %+v", after)
	}
}

// TestAuthDeps_SaveCfg_LoadError verifies a malformed existing file surfaces
// as an error from SaveCfg rather than silently clobbering the user's data.
func TestAuthDeps_SaveCfg_LoadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	deps := authDeps(client, func(string) string { return "" }, cfgPath, "default")
	if err := deps.SaveCfg(auth.Config{Endpoint: "e", Token: "t"}); err == nil {
		t.Fatal("expected error on malformed existing config")
	}
}

// TestAuthDeps_LoadCfg_LoadError mirrors the SaveCfg variant — a malformed
// file must surface through LoadCfg rather than silently masking the problem.
func TestAuthDeps_LoadCfg_LoadError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	deps := authDeps(client, func(string) string { return "" }, cfgPath, "default")
	if _, err := deps.LoadCfg(); err == nil {
		t.Fatal("expected error on malformed existing config")
	}
}

// TestAuthDeps_EmptyPath_FallsBackToEnv exercises the cfgPath=="" branches of
// SaveCfg/ConfigPath: with XDG_CONFIG_HOME set, both should resolve a path
// under that directory rather than erroring.
func TestAuthDeps_EmptyPath_FallsBackToEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	deps := authDeps(client, env, "", "default")

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
	t.Parallel()
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	verbs := buildVerbs(client, func(string) string { return "" }, "", "default", "https://example")
	want := []string{"api", "auth", "profile", "org", "connector", "chat", "dashboard", "playbook", "ontology", "feed", "audit", "version", "update"}
	for _, v := range want {
		if _, ok := verbs[v]; !ok {
			t.Errorf("missing verb: %q", v)
		}
	}
	if len(verbs) != len(want) {
		t.Errorf("verb count = %d, want %d (verbs=%v)", len(verbs), len(want), verbs)
	}
}

// TestProfileDeps_LoadSave_RoundTrip drives profileDeps against a real
// on-disk config file in t.TempDir(), exercising LoadCfg, SaveCfg, and
// ConfigPath. Mirrors TestAuthDeps_LoadSave_RoundTrip — the profile verb
// speaks config.Config directly so the assertions are simpler (no projection
// in the middle).
func TestProfileDeps_LoadSave_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	seed := config.Config{
		Profiles: map[string]config.Profile{
			"default": {Endpoint: "https://existing", Token: "t0", OrgName: "Existing Co"},
		},
		Active: "default",
	}
	if err := config.Save(cfgPath, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	env := func(string) string { return "" }

	deps := profileDeps(env, cfgPath)

	got, err := deps.LoadCfg()
	if err != nil {
		t.Fatalf("LoadCfg: %v", err)
	}
	if got.Active != "default" || got.Profiles["default"].Endpoint != "https://existing" {
		t.Fatalf("LoadCfg round-trip: %+v", got)
	}

	got.Upsert("alt", config.Profile{Endpoint: "https://alt", Token: "t1", OrgName: "Alt"})
	if err := deps.SaveCfg(got); err != nil {
		t.Fatalf("SaveCfg: %v", err)
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := after.Profiles["alt"]; !ok {
		t.Fatalf("alt missing after save: %+v", after)
	}

	pp, err := deps.ConfigPath()
	if err != nil || pp != cfgPath {
		t.Fatalf("ConfigPath: p=%q err=%v", pp, err)
	}
}

// TestProfileDeps_EmptyPath_FallsBackToEnv exercises the cfgPath=="" branches
// of all three closures: with XDG_CONFIG_HOME set they must resolve into
// that directory rather than erroring.
func TestProfileDeps_EmptyPath_FallsBackToEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	deps := profileDeps(env, "")

	// LoadCfg on a missing file returns zero config, no error.
	if _, err := deps.LoadCfg(); err != nil {
		t.Fatalf("LoadCfg: %v", err)
	}
	p, err := deps.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if !strings.HasPrefix(p, dir) {
		t.Fatalf("ConfigPath did not honor XDG: %q", p)
	}
	c := config.Config{
		Profiles: map[string]config.Profile{"x": {Endpoint: "https://e", Token: "t"}},
		Active:   "x",
	}
	if err := deps.SaveCfg(c); err != nil {
		t.Fatalf("SaveCfg: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file at %q: %v", p, err)
	}
}

// TestProfileDeps_EmptyPath_NoEnv covers the error branches: with neither
// cfgPath nor any env var set, all three closures must surface the
// config.DefaultPath error rather than silently writing to nowhere.
func TestProfileDeps_EmptyPath_NoEnv(t *testing.T) {
	t.Parallel()
	deps := profileDeps(func(string) string { return "" }, "")
	if _, err := deps.LoadCfg(); err == nil {
		t.Fatal("LoadCfg: expected error with no HOME/XDG set")
	}
	if err := deps.SaveCfg(config.Config{}); err == nil {
		t.Fatal("SaveCfg: expected error with no HOME/XDG set")
	}
	if _, err := deps.ConfigPath(); err == nil {
		t.Fatal("ConfigPath: expected error with no HOME/XDG set")
	}
}

// TestChatDeps_Fields smoke-tests chatDeps wiring. We don't invoke Stream here
// (that's covered by TestStreamAdapter_*), just assert the deps struct is
// populated.
func TestChatDeps_Fields(t *testing.T) {
	t.Parallel()
	client := transport.New("https://example", func(context.Context) (string, error) { return "", nil })
	d := chatDeps(client)
	if d.Unary == nil || d.Stream == nil || d.UUIDFn == nil {
		t.Fatalf("chatDeps missing fields: %+v", d)
	}
	if u := d.UUIDFn(); !uuidRe.MatchString(u) {
		t.Fatalf("UUIDFn returned non-canonical: %q", u)
	}
}

// TestVersionCmd_PrintsBanner covers the `ana version` verb itself: the
// module-level version/commit/date vars should appear in the output, and Run
// should return nil (exit 0).
func TestVersionCmd_PrintsBanner(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	stdio := cli.IO{Stdout: &out, Stderr: &bytes.Buffer{}}
	if err := (versionCmd{}).Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "ana version") {
		t.Fatalf("expected banner, got %q", out.String())
	}
}

// TestVersionCmd_Help covers the --help short-circuit.
func TestVersionCmd_Help(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	stdio := cli.IO{Stdout: &out, Stderr: &bytes.Buffer{}}
	err := (versionCmd{}).Run(context.Background(), []string{"--help"}, stdio)
	if !errors.Is(err, cli.ErrHelp) {
		t.Fatalf("err = %v, want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "Print ana version") {
		t.Fatalf("help body missing: %q", out.String())
	}
}

// TestRun_VersionFlag exercises the --version rewrite in run(): passing the
// flag at the top level must route through the `version` verb and print the
// banner on stdout with exit code 0.
func TestRun_VersionFlag(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	err := run([]string{"--version"}, stdio, func(string) string { return "" })
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "ana version") {
		t.Fatalf("expected banner, got %q", out.String())
	}
}

// TestRun_NoArgs_PrintsHelp verifies the top-level behavior: no args prints
// root help to stdout and returns ErrHelp (exit code 0 via cli.ExitCode).
func TestRun_NoArgs_PrintsHelp(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	err := run(nil, stdio, func(string) string { return "" })
	if cli.ExitCode(err) != 0 {
		t.Fatalf("exit code = %d, want 0 (err=%v)", cli.ExitCode(err), err)
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
	t.Parallel()
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

// TestRun_LeafUsageErrorReturned asserts that an unknown flag on a leaf verb
// surfaces as a non-nil ErrUsage-wrapped error whose message names the leaf
// and the stdlib "flag provided but not defined" cause. main() then prints
// the error text on stderr — previously it was swallowed because main skipped
// any ErrUsage-wrapping error as "already reported", but leaf FlagSets use
// io.Discard output so nothing had actually been reported.
func TestRun_LeafUsageErrorReturned(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	err := run([]string{"org", "show", "--no-such-flag"}, stdio, func(string) string { return "" })
	if err == nil {
		t.Fatalf("want non-nil error")
	}
	if cli.ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", cli.ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err missing stdlib message: %v", err)
	}
	if !strings.Contains(err.Error(), "show") {
		t.Errorf("err missing leaf name: %v", err)
	}
}

// TestStartNudge_SkipConditions covers every reason startNudge returns nil:
// dev version, --json, interval disabled, no HOME/XDG. Each skip must short-
// circuit before the goroutine spawns, which we assert by the returned ch
// being nil.
func TestStartNudge_SkipConditions(t *testing.T) {
	// Mutates the package-level version var — must not run in parallel with
	// TestVersionCmd_PrintsBanner or TestRun_VersionFlag, both of which read
	// it concurrently under -race.
	prev := version
	t.Cleanup(func() { version = prev })
	envNone := func(string) string { return "" }
	envHome := func(k string) string {
		if k == "HOME" {
			return t.TempDir()
		}
		return ""
	}
	disable := "disable"
	cases := []struct {
		name    string
		version string
		env     func(string) string
		cfg     config.Config
		global  cli.Global
	}{
		{"dev build", "dev", envHome, config.Config{}, cli.Global{}},
		{"json output", "1.0.0", envHome, config.Config{}, cli.Global{JSON: true}},
		{"disabled", "1.0.0", envHome, config.Config{UpdateCheckInterval: &disable}, cli.Global{}},
		{"no home", "1.0.0", envNone, config.Config{}, cli.Global{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version = tc.version
			if got := startNudge(tc.env, tc.cfg, tc.global); got != nil {
				t.Fatalf("expected nil channel, got %v", got)
			}
		})
	}
}

// TestDrainNudge covers the four branches: nil channel, help-err suppression,
// non-empty message printed, and empty message (no print).
func TestDrainNudge(t *testing.T) {
	t.Parallel()
	t.Run("nil channel is a no-op", func(t *testing.T) {
		var buf bytes.Buffer
		drainNudge(nil, time.Millisecond, nil, &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("help err suppresses", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "should not print"
		var buf bytes.Buffer
		drainNudge(ch, time.Millisecond, cli.ErrHelp, &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("message printed", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "hello"
		var buf bytes.Buffer
		drainNudge(ch, time.Millisecond, nil, &buf)
		if !strings.Contains(buf.String(), "hello") {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("empty message swallowed", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- ""
		var buf bytes.Buffer
		drainNudge(ch, time.Millisecond, nil, &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("timeout", func(t *testing.T) {
		ch := make(chan string) // no sender
		var buf bytes.Buffer
		drainNudge(ch, 10*time.Millisecond, nil, &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
}

// TestUpdateCmd_Help short-circuits on --help like every other leaf verb.
func TestUpdateCmd_Help(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	stdio := cli.IO{Stdout: &out, Stderr: &bytes.Buffer{}}
	err := (updateCmd{}).Run(context.Background(), []string{"--help"}, stdio)
	if !errors.Is(err, cli.ErrHelp) {
		t.Fatalf("err = %v, want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "latest ana release") {
		t.Fatalf("help body missing: %q", out.String())
	}
}

// TestRun_UnknownProfile drives the ErrUnknownProfile branch in run: a
// --profile pointing at a slot that doesn't exist (and no env fallback)
// must print the canonical error to stderr and exit 1 via ErrUsage.
func TestRun_UnknownProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ana", "config.json")
	seed := config.Config{
		Profiles: map[string]config.Profile{"default": {Endpoint: "https://x", Token: "t"}},
		Active:   "default",
	}
	if err := config.Save(cfgPath, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: env, Now: time.Now}
	err := run([]string{"--profile", "ghost", "connector", "list"}, stdio, env)
	if cli.ExitCode(err) != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", cli.ExitCode(err), err)
	}
	if !strings.Contains(errb.String(), `unknown profile "ghost"`) {
		t.Errorf("stderr missing message: %q", errb.String())
	}
	// run() writes the diagnostic itself and returns an ErrReported-wrapped
	// err so main's fallback print is suppressed. Regression guard — if
	// ErrReported leaks off the error, users would see the same message
	// twice. Confirms the sentinel is actually in the chain.
	if !errors.Is(err, cli.ErrReported) {
		t.Errorf("err should carry cli.ErrReported to suppress main's duplicate print: %v", err)
	}
}
