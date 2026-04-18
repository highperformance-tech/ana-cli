# internal/config

Reads and writes the ana CLI config file at `$XDG_CONFIG_HOME/ana/config.json` (falling back to `~/.config/ana/config.json`). The on-disk shape holds one or more named profiles, each carrying an endpoint + token pair; a single `active` pointer selects the default. API keys are org-scoped, so users targeting multiple TextQL orgs keep one profile per org.

## Files

- `config.go` — `Profile`, `Config`, `DefaultPath`, `Load`, `Save`, `ActiveProfile`, `Upsert`, and `Resolve` (the endpoint + token + profile-name resolver that merges env fallbacks with the loaded config). Owns `DefaultEndpoint` and `ErrUnknownProfile`.
- `config_test.go` — covers the full round-trip (Load/Save/Upsert) in a `t.TempDir`, profile resolution precedence (flag > env > config > default), and the `ErrUnknownProfile` path.
