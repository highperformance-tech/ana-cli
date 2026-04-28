# internal/config

Reads and writes the ana CLI config file at `$XDG_CONFIG_HOME/ana/config.json` (falling back to `~/.config/ana/config.json`). The on-disk shape holds one or more named profiles, each carrying an endpoint + `cli.Token` pair (the redacting string type — see `internal/cli/token.go`); a single `active` pointer selects the default. API keys are org-scoped, so users targeting multiple TextQL orgs keep one profile per org.

## Files

- `config.go` — `Profile`, `Config`, the `Load`/`Save`/`Upsert`/`Remove`/`ActiveProfile` operations, and `Resolve` (flag > env > config > default precedence). Owns `DefaultEndpoint`, `ErrUnknownProfile`, and the optional `UpdateCheckInterval *string` (omitempty; consumed by `internal/update.ParseInterval`).
- `config_test.go` — full round-trip + precedence coverage in a `t.TempDir`.
