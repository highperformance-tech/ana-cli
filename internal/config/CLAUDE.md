# internal/config

Reads and writes the ana CLI config file at `$XDG_CONFIG_HOME/ana/config.json` (falling back to `~/.config/ana/config.json`). The on-disk shape holds one or more named profiles, each carrying an endpoint + `cli.Token` pair (the redacting string type — see `internal/cli/token.go`); a single `active` pointer selects the default. API keys are org-scoped, so users targeting multiple TextQL orgs keep one profile per org.

## Files

- `config.go` — `Profile`, `Config`, `DefaultPath`, `Load`, `Save`, `ActiveProfile`, `Upsert`, `Remove`, and `Resolve` (the endpoint + token + profile-name resolver that merges env fallbacks with the loaded config). Owns `DefaultEndpoint` and `ErrUnknownProfile`. Also carries the optional `UpdateCheckInterval *string` pointer (interpreted by `internal/update.ParseInterval`) — `omitempty` so existing config files stay unchanged.
- `config_test.go` — covers the full round-trip (Load/Save/Upsert) in a `t.TempDir`, profile resolution precedence (flag > env > config > default), and the `ErrUnknownProfile` path.
