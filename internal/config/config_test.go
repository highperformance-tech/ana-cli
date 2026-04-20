package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sameConfig reports whether two Config values carry identical Profile maps
// and Active pointers. Replaces reflect.DeepEqual now that Profile is a
// string-only struct (so Profile is comparable and maps.Equal suffices).
func sameConfig(a, b Config) bool {
	return a.Active == b.Active && maps.Equal(a.Profiles, b.Profiles)
}

// envMap builds an env lookup function backed by a map.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultPath_XDG(t *testing.T) {
	t.Parallel()
	got, err := DefaultPath(envMap(map[string]string{
		"XDG_CONFIG_HOME": "/xdg",
		"HOME":            "/home/u",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/xdg", "ana", "config.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultPath_HomeFallback(t *testing.T) {
	t.Parallel()
	got, err := DefaultPath(envMap(map[string]string{"HOME": "/home/u"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/home/u", ".config", "ana", "config.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultPath_NeitherSet(t *testing.T) {
	t.Parallel()
	_, err := DefaultPath(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "XDG_CONFIG_HOME") {
		t.Errorf("error should mention XDG_CONFIG_HOME: %v", err)
	}
}

func TestLoad_Missing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nope.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if cfg.Profiles != nil || cfg.Active != "" {
		t.Errorf("expected zero Config, got %+v", cfg)
	}
}

func TestLoad_ValidNewShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := Config{
		Profiles: map[string]Profile{
			"default": {Endpoint: "https://example.com", Token: "abc"},
			"prod":    {Endpoint: "https://prod", Token: "ptok", OrgName: "Prod Co"},
		},
		Active: "default",
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sameConfig(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoad_Malformed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got %v", err)
	}
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		t.Errorf("expected wrapped *json.SyntaxError, got %v", err)
	}
}

// TestLoad_MalformedProfilesBody covers the second json.Unmarshal branch in
// Load: top-level object parses (so the probe succeeds and "profiles" key is
// present), but the Profiles value itself is the wrong type.
func TestLoad_MalformedProfilesBody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// "profiles" is a string, not an object — second Unmarshal will fail.
	if err := os.WriteFile(path, []byte(`{"profiles":"nope"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

// TestLoad_MalformedLegacyBody covers the legacy-branch second Unmarshal: the
// top-level object parses and has an "endpoint" key, but its value is not a
// string.
func TestLoad_MalformedLegacyBody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"endpoint":123}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLoad_UnreadablePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("should not be ErrNotExist: %v", err)
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("expected read error, got %v", err)
	}
}

func TestLoad_LegacyMigrationBothFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"endpoint":"https://legacy","token":"lt"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := Config{
		Profiles: map[string]Profile{"default": {Endpoint: "https://legacy", Token: "lt"}},
		Active:   "default",
	}
	if !sameConfig(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestLoad_LegacyMigrationEndpointOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"endpoint":"https://legacy"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p, ok := got.Profiles["default"]; !ok || p.Endpoint != "https://legacy" || p.Token != "" {
		t.Fatalf("got %+v", got)
	}
	if got.Active != "default" {
		t.Errorf("active = %q, want default", got.Active)
	}
}

func TestLoad_LegacyMigrationTokenOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"token":"ltok"}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p, ok := got.Profiles["default"]; !ok || p.Token != "ltok" {
		t.Fatalf("got %+v", got)
	}
}

// TestLoad_EmptyObject covers the "no profiles key, no legacy fields" branch
// which must return zero Config.
func TestLoad_EmptyObject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Profiles != nil || got.Active != "" {
		t.Errorf("expected zero Config, got %+v", got)
	}
}

func TestLoad_InferActiveWhenSingleProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := []byte(`{"profiles":{"only":{"endpoint":"https://x","token":"t"}},"active":""}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Active != "only" {
		t.Errorf("active = %q, want only", got.Active)
	}
}

func TestLoad_MultipleProfilesNoActive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := []byte(`{"profiles":{"a":{"endpoint":"u"},"b":{"endpoint":"v"}},"active":""}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Active != "" {
		t.Errorf("active should remain empty, got %q", got.Active)
	}
}

// TestLoad_ActiveMissingProfileKept verifies that Load does NOT silently fix
// up a dangling Active pointer; Resolve is the layer that surfaces the error.
func TestLoad_ActiveMissingProfileKept(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := []byte(`{"profiles":{"a":{"endpoint":"u"}},"active":"ghost"}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Active != "ghost" {
		t.Errorf("active = %q, want ghost (unchanged)", got.Active)
	}
}

func TestSave_CreatesDirAndFileWithModes(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	path := filepath.Join(base, "nested", "dir", "config.json")
	cfg := Config{
		Profiles: map[string]Profile{"default": {Endpoint: "https://example.com", Token: "xyz"}},
		Active:   "default",
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %o, want 0600", fi.Mode().Perm())
		}
		di, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatalf("stat dir: %v", err)
		}
		if di.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %o, want 0700", di.Mode().Perm())
		}
	}
}

func TestSave_NoTempFileRemains(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Config{Profiles: map[string]Profile{"default": {Endpoint: "e"}}, Active: "default"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected no .tmp file, stat err = %v", err)
	}
}

func TestSave_MkdirFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	path := filepath.Join(blocker, "sub", "config.json")
	err := Save(path, Config{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mkdir") {
		t.Errorf("expected mkdir error, got %v", err)
	}
}

func TestSave_RenameFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "x"), []byte("x"), 0o600); err != nil {
		t.Fatalf("populate dest: %v", err)
	}
	err := Save(path, Config{Profiles: map[string]Profile{"d": {Endpoint: "e"}}, Active: "d"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Errorf("expected rename error, got %v", err)
	}
	if _, statErr := os.Stat(path + ".tmp"); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("expected .tmp cleaned up, got %v", statErr)
	}
}

func TestSave_WriteFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(path+".tmp", 0o700); err != nil {
		t.Fatalf("mkdir tmp dir: %v", err)
	}
	err := Save(path, Config{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "write") {
		t.Errorf("expected write error, got %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")
	want := Config{
		Profiles: map[string]Profile{
			"default": {Endpoint: "https://example.com", Token: "secret", OrgName: "Acme"},
		},
		Active: "default",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !sameConfig(got, want) {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

func TestActiveProfile(t *testing.T) {
	t.Parallel()
	// Active set and present.
	c := Config{
		Profiles: map[string]Profile{"a": {Endpoint: "e", Token: "t"}},
		Active:   "a",
	}
	name, p, ok := c.ActiveProfile()
	if !ok || name != "a" || p.Endpoint != "e" {
		t.Errorf("present: got name=%q p=%+v ok=%v", name, p, ok)
	}
	// Active set but missing.
	c = Config{Profiles: map[string]Profile{"a": {}}, Active: "ghost"}
	if _, _, ok := c.ActiveProfile(); ok {
		t.Errorf("missing active should return ok=false")
	}
	// Active empty.
	c = Config{Profiles: map[string]Profile{"a": {}}}
	if _, _, ok := c.ActiveProfile(); ok {
		t.Errorf("empty active should return ok=false")
	}
}

func TestUpsert_NewMap(t *testing.T) {
	t.Parallel()
	var c Config
	c.Upsert("default", Profile{Endpoint: "e", Token: "t"})
	if c.Profiles["default"].Endpoint != "e" {
		t.Errorf("profile not stored: %+v", c)
	}
	if c.Active != "default" {
		t.Errorf("active should auto-set to default, got %q", c.Active)
	}
}

func TestUpsert_ExistingOverrides(t *testing.T) {
	t.Parallel()
	c := Config{
		Profiles: map[string]Profile{"a": {Endpoint: "old"}},
		Active:   "a",
	}
	c.Upsert("a", Profile{Endpoint: "new", Token: "t"})
	if got := c.Profiles["a"]; got.Endpoint != "new" || got.Token != "t" {
		t.Errorf("upsert did not replace: %+v", got)
	}
	if c.Active != "a" {
		t.Errorf("active should not change: %q", c.Active)
	}
}

func TestUpsert_LeavesActiveAloneWhenSet(t *testing.T) {
	t.Parallel()
	c := Config{
		Profiles: map[string]Profile{"existing": {}},
		Active:   "existing",
	}
	c.Upsert("new", Profile{Endpoint: "e"})
	if c.Active != "existing" {
		t.Errorf("active clobbered: %q", c.Active)
	}
}

func TestRemove_Missing(t *testing.T) {
	t.Parallel()
	c := Config{Profiles: map[string]Profile{"a": {}}, Active: "a"}
	if c.Remove("ghost") {
		t.Fatal("Remove should return false for missing")
	}
	if _, ok := c.Profiles["a"]; !ok {
		t.Errorf("existing profile lost")
	}
}

func TestRemove_NonActive(t *testing.T) {
	t.Parallel()
	c := Config{
		Profiles: map[string]Profile{"a": {}, "b": {}},
		Active:   "a",
	}
	if !c.Remove("b") {
		t.Fatal("Remove should return true")
	}
	if _, ok := c.Profiles["b"]; ok {
		t.Error("b should be gone")
	}
	if c.Active != "a" {
		t.Errorf("active should stay 'a', got %q", c.Active)
	}
}

func TestRemove_ActiveDeterministicReplacement(t *testing.T) {
	t.Parallel()
	c := Config{
		Profiles: map[string]Profile{"z": {}, "a": {}, "m": {}},
		Active:   "z",
	}
	if !c.Remove("z") {
		t.Fatal("Remove should return true")
	}
	// Lex-first of {a, m} is "a".
	if c.Active != "a" {
		t.Errorf("active = %q, want a", c.Active)
	}
}

func TestRemove_LastProfileClearsActive(t *testing.T) {
	t.Parallel()
	c := Config{Profiles: map[string]Profile{"only": {}}, Active: "only"}
	if !c.Remove("only") {
		t.Fatal("Remove should return true")
	}
	if c.Active != "" {
		t.Errorf("active should be cleared, got %q", c.Active)
	}
}

func TestResolve_ExplicitProfileWins(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{
			"default": {Endpoint: "https://default", Token: "dtok"},
			"prod":    {Endpoint: "https://prod", Token: "ptok"},
		},
		Active: "default",
	}
	env := envMap(map[string]string{"ANA_PROFILE": "ignored"})
	p, name, err := Resolve(env, loaded, "prod")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "prod" || p.Endpoint != "https://prod" || p.Token != "ptok" {
		t.Errorf("got name=%q p=%+v", name, p)
	}
}

func TestResolve_EnvProfileWinsOverActive(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{
			"default": {Endpoint: "https://default"},
			"staging": {Endpoint: "https://staging"},
		},
		Active: "default",
	}
	env := envMap(map[string]string{"ANA_PROFILE": "staging"})
	p, name, err := Resolve(env, loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "staging" || p.Endpoint != "https://staging" {
		t.Errorf("got name=%q p=%+v", name, p)
	}
}

func TestResolve_ActiveUsed(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{"default": {Endpoint: "https://d", Token: "t"}},
		Active:   "default",
	}
	p, name, err := Resolve(envMap(map[string]string{}), loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "default" || p.Token != "t" {
		t.Errorf("got name=%q p=%+v", name, p)
	}
}

func TestResolve_FirstSortedKeyFallback(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{
			"zed":   {Endpoint: "https://z"},
			"alpha": {Endpoint: "https://a"},
		},
		// Active intentionally empty — forces the sorted-key fallback.
	}
	p, name, err := Resolve(envMap(map[string]string{}), loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "alpha" || p.Endpoint != "https://a" {
		t.Errorf("got name=%q p=%+v", name, p)
	}
}

func TestResolve_DefaultNameFallback(t *testing.T) {
	t.Parallel()
	// No profiles at all: picks "default", returns empty profile with
	// DefaultEndpoint filled in.
	p, name, err := Resolve(envMap(map[string]string{}), Config{}, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "default" {
		t.Errorf("name = %q, want default", name)
	}
	if p.Endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want DefaultEndpoint", p.Endpoint)
	}
}

func TestResolve_EnvOverridesApplied(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{"default": {Endpoint: "https://loaded", Token: "lt"}},
		Active:   "default",
	}
	env := envMap(map[string]string{
		"ANA_ENDPOINT": "https://env",
		"ANA_TOKEN":    "env-tok",
	})
	p, _, err := Resolve(env, loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if p.Endpoint != "https://env" || p.Token != "env-tok" {
		t.Errorf("got %+v", p)
	}
}

func TestResolve_DefaultEndpointFilledWhenEmpty(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{"default": {Token: "t"}},
		Active:   "default",
	}
	p, _, err := Resolve(envMap(map[string]string{}), loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if p.Endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q", p.Endpoint)
	}
}

func TestResolve_UnknownProfileError(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{"default": {Endpoint: "e"}},
		Active:   "default",
	}
	_, name, err := Resolve(envMap(map[string]string{}), loaded, "ghost")
	if !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("err=%v, want ErrUnknownProfile", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err should mention 'ghost': %v", err)
	}
	if name != "ghost" {
		t.Errorf("name = %q, want ghost", name)
	}
}

// TestResolve_UnknownProfileButEnvBypass covers the bypass: explicit --profile
// pointing at a non-existent slot is fine when ANA_ENDPOINT/TOKEN override.
// This matches the first-run login use case.
func TestResolve_UnknownProfileButEnvBypass(t *testing.T) {
	t.Parallel()
	env := envMap(map[string]string{"ANA_ENDPOINT": "https://env", "ANA_TOKEN": "env-tok"})
	p, name, err := Resolve(env, Config{}, "fresh")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "fresh" || p.Endpoint != "https://env" || p.Token != "env-tok" {
		t.Errorf("got name=%q p=%+v", name, p)
	}
}

// TestResolve_ImplicitMissingProfileNoError exercises the case where no
// --profile flag was passed, Active points at something missing, and no env
// fallback. The contract says this returns an empty profile (no error) —
// Resolve only errors when the user explicitly asked by name.
func TestResolve_ImplicitMissingProfileNoError(t *testing.T) {
	t.Parallel()
	loaded := Config{
		Profiles: map[string]Profile{"a": {Endpoint: "https://a"}},
		Active:   "ghost", // dangling
	}
	p, name, err := Resolve(envMap(map[string]string{}), loaded, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if name != "ghost" {
		t.Errorf("name = %q, want ghost", name)
	}
	if p.Endpoint != DefaultEndpoint {
		t.Errorf("endpoint should default-fill, got %q", p.Endpoint)
	}
}
