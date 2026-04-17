// Package config reads and writes the ana CLI configuration file.
//
// The on-disk shape holds one or more named profiles, each carrying an
// endpoint + token pair. API keys are org-scoped, so users targeting more
// than one TextQL org need one profile per org. A single "active" pointer
// selects which profile is used by default.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DefaultEndpoint is used by Resolve when no endpoint is configured.
const DefaultEndpoint = "https://app.textql.com"

// ErrUnknownProfile is returned by Resolve when the caller explicitly asked
// for a profile by name (via --profile) that does not exist in the loaded
// config AND no ANA_ENDPOINT/ANA_TOKEN env fallback was provided. Wrapped
// with %w so callers can detect it via errors.Is.
var ErrUnknownProfile = errors.New("config: unknown profile")

// Profile is a single named {endpoint, token} pair. OrgName is a human label
// captured at login time; it is purely informational and never affects
// resolution.
type Profile struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	OrgName  string `json:"orgName,omitempty"`
}

// Config is the persisted CLI configuration. Profiles maps profile name to
// its Profile. Active names the profile selected by default.
type Config struct {
	Profiles map[string]Profile `json:"profiles"`
	Active   string             `json:"active"`
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

// legacyConfig mirrors the pre-multi-profile shape {endpoint, token}. We
// decode into a separate type so we can distinguish "this is an old file"
// from "this is a new file with no profiles yet" without guessing.
type legacyConfig struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

// Load reads a config file from path. A missing file yields a zero-value
// Config and a nil error (first-run case). Other IO or JSON errors are
// wrapped and returned.
//
// If the file is a legacy {endpoint, token} document (no "profiles" key), it
// is migrated in-memory to a single "default" profile. The migration is not
// persisted automatically — the next Save overwrites in the new shape.
//
// When the new shape is present with an empty Active and exactly one
// profile, Active is inferred from that profile's name. With two or more
// profiles Active stays empty and callers must pick one.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	// Detect legacy vs new shape by looking for the "profiles" key. We parse
	// into a generic map first because the two shapes are disjoint: the new
	// shape has "profiles"; the old one has "endpoint"/"token" at top level.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if _, hasProfiles := probe["profiles"]; !hasProfiles {
		// Either legacy or an empty/partial new file. If there are any
		// legacy-shaped fields, migrate. Otherwise return zero.
		_, hasEndpoint := probe["endpoint"]
		_, hasToken := probe["token"]
		if hasEndpoint || hasToken {
			var legacy legacyConfig
			if err := json.Unmarshal(data, &legacy); err != nil {
				return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
			}
			return Config{
				Profiles: map[string]Profile{
					"default": {Endpoint: legacy.Endpoint, Token: legacy.Token},
				},
				Active: "default",
			}, nil
		}
		return Config{}, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Infer Active when only one profile is present — a one-profile file
	// with no explicit active pointer is unambiguous.
	if cfg.Active == "" && len(cfg.Profiles) == 1 {
		for name := range cfg.Profiles {
			cfg.Active = name
		}
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
	// Config holds only strings + a map of strings; json.MarshalIndent cannot
	// fail for these types.
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

// ActiveProfile returns the currently active profile. ok is false when
// Active is unset or points at a profile that is not in the map.
func (c Config) ActiveProfile() (string, Profile, bool) {
	if c.Active == "" {
		return "", Profile{}, false
	}
	p, ok := c.Profiles[c.Active]
	if !ok {
		return "", Profile{}, false
	}
	return c.Active, p, true
}

// Upsert inserts or replaces a profile by name. When Active is empty it is
// set to name so the first profile written always becomes active. Callers
// must Save afterwards to persist.
func (c *Config) Upsert(name string, p Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]Profile)
	}
	c.Profiles[name] = p
	if c.Active == "" {
		c.Active = name
	}
}

// Remove deletes a profile by name. When the removed profile was Active, a
// replacement is chosen deterministically (lexicographic first of whatever
// remains); if no profiles remain, Active is cleared. Returns whether
// anything was removed.
func (c *Config) Remove(name string) bool {
	if _, ok := c.Profiles[name]; !ok {
		return false
	}
	delete(c.Profiles, name)
	if c.Active != name {
		return true
	}
	if len(c.Profiles) == 0 {
		c.Active = ""
		return true
	}
	names := make([]string, 0, len(c.Profiles))
	for k := range c.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	c.Active = names[0]
	return true
}

// Resolve selects a profile and applies environment overrides.
//
// Profile selection priority (first non-empty wins):
//  1. profileName argument (from --profile)
//  2. env("ANA_PROFILE")
//  3. loaded.Active
//  4. first key of loaded.Profiles, sorted for determinism
//  5. "default"
//
// A selected name that does not exist in loaded.Profiles yields an empty
// Profile{} — this is not an error because first-run/login writes into a
// named slot that doesn't exist yet. The one exception: when the caller
// passed an explicit profileName that is missing AND no ANA_ENDPOINT /
// ANA_TOKEN env override is present, that's a user mistake (e.g. `--profile
// prod` when `prod` is not configured) and ErrUnknownProfile is returned
// with the bad name in the message.
//
// ANA_ENDPOINT and ANA_TOKEN override whatever was resolved from the
// profile. If the final Endpoint is still empty, DefaultEndpoint fills it.
func Resolve(env func(string) string, loaded Config, profileName string) (Profile, string, error) {
	name := pickProfileName(env, loaded, profileName)
	p, found := loaded.Profiles[name]

	envEndpoint := env("ANA_ENDPOINT")
	envToken := env("ANA_TOKEN")

	// A --profile that doesn't exist is a hard error unless the user is
	// overriding everything via env vars (in which case the profile name is
	// effectively just a slot the next Save will create).
	if !found && profileName != "" && envEndpoint == "" && envToken == "" {
		return Profile{}, name, fmt.Errorf("%w: %q", ErrUnknownProfile, profileName)
	}

	if envEndpoint != "" {
		p.Endpoint = envEndpoint
	}
	if envToken != "" {
		p.Token = envToken
	}
	if p.Endpoint == "" {
		p.Endpoint = DefaultEndpoint
	}
	return p, name, nil
}

// pickProfileName walks the five-level precedence chain in Resolve. Pulled
// out so the logic can be read linearly without env/override noise.
func pickProfileName(env func(string) string, loaded Config, profileName string) string {
	if profileName != "" {
		return profileName
	}
	if v := env("ANA_PROFILE"); v != "" {
		return v
	}
	if loaded.Active != "" {
		return loaded.Active
	}
	if len(loaded.Profiles) > 0 {
		names := make([]string, 0, len(loaded.Profiles))
		for k := range loaded.Profiles {
			names = append(names, k)
		}
		sort.Strings(names)
		return names[0]
	}
	return "default"
}
