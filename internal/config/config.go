// Package config reads and writes the ana CLI configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultEndpoint is used by Resolve when no endpoint is configured.
const DefaultEndpoint = "https://app.textql.com"

// Config is the persisted CLI configuration.
type Config struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

// DefaultPath returns the default path for the config file.
//
// It prefers $XDG_CONFIG_HOME/ana/config.json; falling back to
// $HOME/.config/ana/config.json. If neither variable is set, an error is
// returned.
//
// env is the environment lookup function (injected so tests do not touch the
// real process environment); callers typically pass os.Getenv.
func DefaultPath(env func(string) string) (string, error) {
	if xdg := env("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ana", "config.json"), nil
	}
	if home := env("HOME"); home != "" {
		return filepath.Join(home, ".config", "ana", "config.json"), nil
	}
	return "", errors.New("config: neither XDG_CONFIG_HOME nor HOME is set")
}

// Load reads a config file from path. A missing file yields a zero-value
// Config and a nil error (first-run case). Other IO or JSON errors are
// wrapped and returned.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path atomically. The parent directory is created with
// mode 0700 if it does not exist; the file itself is written with mode 0600.
// Writes go through path+".tmp" followed by os.Rename.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	// Config holds only strings; json.MarshalIndent cannot fail here.
	data, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("config: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("config: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Resolve merges the loaded config with environment overrides.
//
// ANA_ENDPOINT overrides Endpoint; ANA_TOKEN overrides Token. Empty or unset
// env vars leave the loaded value in place. If both the loaded Endpoint and
// the env override are empty, DefaultEndpoint is used.
func Resolve(env func(string) string, loaded Config) Config {
	out := loaded
	if v := env("ANA_ENDPOINT"); v != "" {
		out.Endpoint = v
	}
	if v := env("ANA_TOKEN"); v != "" {
		out.Token = v
	}
	if out.Endpoint == "" {
		out.Endpoint = DefaultEndpoint
	}
	return out
}
