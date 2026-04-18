# internal/auth

The `ana auth` verb tree: `login`, `logout`, `whoami`, plus nested `keys` and `service-accounts` groups. Pure dispatch logic around an injected `Deps` struct (Unary RPC + config load/save closures) so the package never imports `internal/transport` or `internal/config` directly. Declares its own narrow `Config` (endpoint + token) to keep the contract narrow.

## Files

- `auth.go` — `New`, `Deps`, local `Config` projection, `DefaultEndpoint`.
- `login.go` / `logout.go` — persist the bearer token to the active profile slot (login) or blank it out (logout).
- `whoami.go` — calls `PublicAuthService.GetMember` + resolves the active org; help text notes the org-scoping caveat.
- `keys.go` — nested group: `ana auth keys create/rotate/revoke/list`. `CreateApiKey` returns the plaintext once as `apiKeyHash`; the command prints it and exits.
- `service_accounts.go` — nested group: `ana auth service-accounts create/delete/list`.
- `flags.go` — shared flag registration helpers for the subcommands.
- `errors.go` — `ErrNotLoggedIn` (`cli.ExitCode` maps to 2), plus the `Error`/`Unwrap`/`IsAuthError` helpers.
- `auth_test.go` — covers every subcommand end-to-end with fake `Deps` closures against a `t.TempDir` config.
