package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// envMap builds an env lookup function backed by a map.
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultPath_XDG(t *testing.T) {
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
	_, err := DefaultPath(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "XDG_CONFIG_HOME") {
		t.Errorf("error should mention XDG_CONFIG_HOME: %v", err)
	}
}

func TestLoad_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if cfg != (Config{}) {
		t.Errorf("expected zero Config, got %+v", cfg)
	}
}

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := Config{Endpoint: "https://example.com", Token: "abc"}
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
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoad_Malformed(t *testing.T) {
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
	// Ensure wrapping preserves the underlying JSON error type.
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		t.Errorf("expected wrapped *json.SyntaxError, got %v", err)
	}
}

func TestLoad_UnreadablePath(t *testing.T) {
	// A directory cannot be read as a regular file; os.ReadFile returns a
	// non-ErrNotExist error that should be wrapped.
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

func TestSave_CreatesDirAndFileWithModes(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "nested", "dir", "config.json")
	cfg := Config{Endpoint: "https://example.com", Token: "xyz"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	// File exists with 0600.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %o, want 0600", fi.Mode().Perm())
		}
		// Parent dir 0700.
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
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Save(path, Config{Endpoint: "e"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected no .tmp file, stat err = %v", err)
	}
}

func TestSave_MkdirFails(t *testing.T) {
	// Make a regular file and try to save into a path whose parent would
	// need to be a subdirectory of that file. MkdirAll fails because the
	// intermediate component is a file, not a directory.
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
	// Rename fails when the destination path is an existing non-empty
	// directory on the same filesystem — os.Rename refuses to replace a
	// directory with a file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	// Put something inside so it's not empty (some platforms allow rename
	// over an empty dir).
	if err := os.WriteFile(filepath.Join(path, "x"), []byte("x"), 0o600); err != nil {
		t.Fatalf("populate dest: %v", err)
	}
	err := Save(path, Config{Endpoint: "e"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rename") {
		t.Errorf("expected rename error, got %v", err)
	}
	// And the .tmp cleanup should have removed the temp file.
	if _, statErr := os.Stat(path + ".tmp"); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("expected .tmp cleaned up, got %v", statErr)
	}
}

func TestSave_WriteFails(t *testing.T) {
	// Force WriteFile to fail: make the .tmp path an existing directory so
	// the write cannot create/truncate a regular file at that path.
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
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.json")
	want := Config{Endpoint: "https://example.com", Token: "secret"}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
}

func TestResolve_EnvOverrides(t *testing.T) {
	loaded := Config{Endpoint: "https://loaded", Token: "loaded-tok"}
	env := envMap(map[string]string{
		"ANA_ENDPOINT": "https://env",
		"ANA_TOKEN":    "env-tok",
	})
	got := Resolve(env, loaded)
	want := Config{Endpoint: "https://env", Token: "env-tok"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolve_EmptyDefaults(t *testing.T) {
	got := Resolve(envMap(map[string]string{}), Config{})
	want := Config{Endpoint: DefaultEndpoint, Token: ""}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolve_LoadedWinsWhenEnvEmpty(t *testing.T) {
	loaded := Config{Endpoint: "https://loaded", Token: "loaded-tok"}
	got := Resolve(envMap(map[string]string{}), loaded)
	if got != loaded {
		t.Errorf("got %+v, want %+v", got, loaded)
	}
}

func TestResolve_EnvWinsOverLoaded(t *testing.T) {
	loaded := Config{Endpoint: "https://loaded", Token: "loaded-tok"}
	env := envMap(map[string]string{
		"ANA_ENDPOINT": "https://env",
		"ANA_TOKEN":    "env-tok",
	})
	got := Resolve(env, loaded)
	want := Config{Endpoint: "https://env", Token: "env-tok"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
