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
)

// uuidRe matches canonical 8-4-4-4-12 lowercase-hex UUIDs.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestNewUUID_Shape verifies the canonical v4 layout.
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
		default:
			t.Fatalf("variant nibble = %q, want one of 8/9/a/b in %q", u[19], u)
		}
	}
}

// TestNewUUID_Unique catches a trivial regression where the generator returns
// a constant.
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

// TestProfileToAuthConfig covers the projection helper.
func TestProfileToAuthConfig(t *testing.T) {
	t.Parallel()
	p := config.Profile{Endpoint: "https://example.com", Token: "abc", OrgName: "Acme"}
	ac := profileToAuthConfig(p)
	if ac.Endpoint != p.Endpoint || ac.Token != p.Token {
		t.Fatalf("profileToAuthConfig lost fields: %+v", ac)
	}
}

// TestLazyState_AuthDeps_RoundTrip drives the auth-deps closures returned by
// lazyState against a real on-disk config in t.TempDir(). Replaces the prior
// per-helper authDeps tests with one round-trip through the same accessor
// the binary uses.
func TestLazyState_AuthDeps_RoundTrip(t *testing.T) {
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
	global := cli.Global{TokenFile: cfgPath}
	state := newLazyState(env, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	deps := state.AuthDeps()

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

// TestLazyState_AuthDeps_DefaultsToNamedSlot covers the empty-profile-name
// fallback: when profileName resolves to "" SaveCfg still writes into
// "default".
func TestLazyState_AuthDeps_DefaultsToNamedSlot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	global := cli.Global{TokenFile: cfgPath}
	state := newLazyState(env, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	deps := state.AuthDeps()
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

// TestLazyState_ProfileDeps_RoundTrip drives the profile-deps closures
// against a real on-disk config.
func TestLazyState_ProfileDeps_RoundTrip(t *testing.T) {
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
	global := cli.Global{TokenFile: cfgPath}
	state := newLazyState(env, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	deps := state.ProfileDeps()

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

// TestLazyState_ProfileDeps_EmptyPath_FallsBackToEnv covers the empty-path
// branches: with XDG_CONFIG_HOME set, all three closures resolve under that
// directory rather than erroring.
func TestLazyState_ProfileDeps_EmptyPath_FallsBackToEnv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	global := cli.Global{}
	state := newLazyState(env, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	deps := state.ProfileDeps()

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

// TestLazyState_ProfileDeps_EmptyPath_NoEnv covers the error branches: with
// neither cfgPath nor any env var set, all three closures surface the
// config.DefaultPath error.
func TestLazyState_ProfileDeps_EmptyPath_NoEnv(t *testing.T) {
	t.Parallel()
	global := cli.Global{}
	state := newLazyState(func(string) string { return "" }, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	deps := state.ProfileDeps()
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

// TestBuildVerbs_Shape checks the top-level verb map registers every verb
// name promised by docs/features.md. Regression guard: a drop would be silent
// otherwise — main.go has no other assertion about verb naming.
func TestBuildVerbs_Shape(t *testing.T) {
	t.Parallel()
	global := cli.Global{}
	state := newLazyState(func(string) string { return "" }, &global, cli.IO{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	verbs := buildVerbs(state)
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

// TestVersionCmd_PrintsBanner covers the `ana version` verb.
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

// TestRun_VersionFlag exercises the --version rewrite in run().
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

// TestRun_NoArgs_PrintsHelp verifies the top-level no-args behavior.
func TestRun_NoArgs_PrintsHelp(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: func(string) string { return "" }, Now: time.Now}
	err := run(nil, stdio, func(string) string { return "" })
	if cli.ExitCode(err) != 0 {
		t.Fatalf("exit code = %d, want 0 (err=%v)", cli.ExitCode(err), err)
	}
	if !strings.Contains(out.String(), "Commands:") {
		t.Fatalf("expected help on stdout, got: %q", out.String())
	}
}

// TestRun_EndToEnd_ConnectorList is the v1 e2e smoke test.
func TestRun_EndToEnd_ConnectorList(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"connectors": []map[string]any{{"id": 1, "name": "alpha"}},
		})
	}))
	defer srv.Close()

	var out, errb bytes.Buffer
	home := t.TempDir()
	envFn := func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	}
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Env: envFn, Now: time.Now}
	args := []string{"--endpoint", srv.URL, "--json", "connector", "list"}
	err := run(args, stdio, envFn)
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
// surfaces as an ErrUsage-wrapped error whose message names the leaf and the
// stdlib "flag provided but not defined" cause.
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
}

// TestStartNudge_SkipConditions covers every reason startNudge returns nil.
func TestStartNudge_SkipConditions(t *testing.T) {
	prev := version
	t.Cleanup(func() { version = prev })
	envNone := func(string) string { return "" }
	envHome := func(k string) string {
		if k == "HOME" {
			return t.TempDir()
		}
		return ""
	}
	cases := []struct {
		name    string
		version string
		env     func(string) string
		global  cli.Global
	}{
		{"dev build", "dev", envHome, cli.Global{}},
		{"json output", "1.0.0", envHome, cli.Global{JSON: true}},
		{"no home", "1.0.0", envNone, cli.Global{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version = tc.version
			if got := startNudge(tc.env, tc.global); got != nil {
				t.Fatalf("expected nil channel, got %v", got)
			}
		})
	}

	// Disabled interval — config writes UpdateCheckInterval="0" so
	// ParseInterval returns enabled=false and startNudge returns nil
	// before launching the goroutine.
	t.Run("disabled interval", func(t *testing.T) {
		version = "1.0.0"
		dir := t.TempDir()
		off := "0"
		cfg := config.Config{UpdateCheckInterval: &off}
		path := filepath.Join(dir, "ana", "config.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := config.Save(path, cfg); err != nil {
			t.Fatal(err)
		}
		env := func(k string) string {
			if k == "XDG_CONFIG_HOME" {
				return dir
			}
			return ""
		}
		if got := startNudge(env, cli.Global{}); got != nil {
			t.Fatalf("expected nil channel for disabled interval, got %v", got)
		}
	})

	// --token-file precedence: a disabled interval written to a non-default
	// path is honored when global.TokenFile points at it, even if the
	// XDG-default config would say otherwise. Pins startNudge to the same
	// path-precedence rule as lazyState.initConfig.
	t.Run("token-file overrides default path", func(t *testing.T) {
		version = "1.0.0"
		dir := t.TempDir()
		altPath := filepath.Join(dir, "alt.json")
		off := "0"
		if err := config.Save(altPath, config.Config{UpdateCheckInterval: &off}); err != nil {
			t.Fatal(err)
		}
		// XDG default points elsewhere with no UpdateCheckInterval — if
		// startNudge ignored TokenFile it would fall back to ParseInterval(nil)
		// → enabled=true and we'd get a non-nil channel.
		xdgDir := t.TempDir()
		defaultPath := filepath.Join(xdgDir, "ana", "config.json")
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := config.Save(defaultPath, config.Config{}); err != nil {
			t.Fatal(err)
		}
		env := func(k string) string {
			if k == "XDG_CONFIG_HOME" {
				return xdgDir
			}
			return ""
		}
		if got := startNudge(env, cli.Global{TokenFile: altPath}); got != nil {
			t.Fatalf("expected nil channel when --token-file points at disabled config, got %v", got)
		}
	})
}

// TestDrainNudge covers every branch. The cancellation arm is exercised by
// passing an already-canceled context; the receive arm by pre-loading the
// buffered channel and passing context.Background(). Both shapes resolve
// deterministically — no wall-clock waits, no select races.
func TestDrainNudge(t *testing.T) {
	t.Parallel()
	t.Run("nil channel is a no-op", func(t *testing.T) {
		var buf bytes.Buffer
		drainNudge(context.Background(), nil, nil, "", &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("help err suppresses", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "should not print"
		var buf bytes.Buffer
		drainNudge(context.Background(), ch, cli.ErrHelp, "", &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("update success suppresses", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "stale nudge from pre-swap version"
		var buf bytes.Buffer
		drainNudge(context.Background(), ch, nil, "update", &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("update failure still nudges", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "retry hint"
		var buf bytes.Buffer
		drainNudge(context.Background(), ch, errors.New("permission denied"), "update", &buf)
		if !strings.Contains(buf.String(), "retry hint") {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("message printed", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- "hello"
		var buf bytes.Buffer
		drainNudge(context.Background(), ch, nil, "", &buf)
		if !strings.Contains(buf.String(), "hello") {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("empty message swallowed", func(t *testing.T) {
		ch := make(chan string, 1)
		ch <- ""
		var buf bytes.Buffer
		drainNudge(context.Background(), ch, nil, "", &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
	t.Run("ctx canceled before message", func(t *testing.T) {
		ch := make(chan string) // unbuffered; never written
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var buf bytes.Buffer
		drainNudge(ctx, ch, nil, "", &buf)
		if buf.Len() != 0 {
			t.Fatalf("stderr: %q", buf.String())
		}
	})
}

// TestFirstVerb covers empty-slice and skip-flag-tokens branches.
func TestFirstVerb(t *testing.T) {
	t.Parallel()
	if got := firstVerb(nil); got != "" {
		t.Errorf("nil slice: got %q, want empty", got)
	}
	if got := firstVerb([]string{}); got != "" {
		t.Errorf("empty slice: got %q, want empty", got)
	}
	if got := firstVerb([]string{"update", "--json"}); got != "update" {
		t.Errorf("got %q, want update", got)
	}
	if got := firstVerb([]string{"--profile", "prod", "org", "list"}); got != "org" {
		t.Errorf("post-flag verb: got %q, want org", got)
	}
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

// TestRun_UnknownProfile drives the ErrUnknownProfile branch in run.
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
	if !errors.Is(err, cli.ErrReported) {
		t.Errorf("err should carry cli.ErrReported to suppress main's duplicate print: %v", err)
	}
}

// TestRun_ProfileAddEndpointPersists is the end-to-end regression test for
// the bug `ana profile add <name> --endpoint X` saved the default endpoint.
// Catches any wiring regression between the resolver, lazyState, and the
// profile package's Flagger leaf.
func TestRun_ProfileAddEndpointPersists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ana", "config.json")
	env := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return dir
		}
		return ""
	}
	stdio := cli.IO{
		Stdin:  strings.NewReader("dummy-token\n"),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Env:    env,
		Now:    time.Now,
	}
	args := []string{"profile", "add", "scratch", "--endpoint", "https://custom.example.com"}
	if err := run(args, stdio, env); err != nil {
		t.Fatalf("run: %v", err)
	}
	c, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	p, ok := c.Profiles["scratch"]
	if !ok {
		t.Fatalf("scratch profile missing: %+v", c)
	}
	if p.Endpoint != "https://custom.example.com" {
		t.Fatalf("endpoint = %q, want https://custom.example.com (resolver should route --endpoint to the leaf, not the global override)", p.Endpoint)
	}
}
