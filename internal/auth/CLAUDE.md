# internal/auth

The `ana auth` verb tree: `login`, `logout`, `whoami`, plus nested `keys` and `service-accounts` groups. Pure dispatch logic around an injected `Deps` struct (Unary RPC + config load/save closures) so the package never imports `internal/transport` or `internal/config` directly. Declares its own narrow `Config` (endpoint + `cli.Token`) to keep the contract narrow; the token field uses the redacting string type so accidental `%v`/`%s` formatting never leaks a bearer token.

## Files

- `auth.go` — `New`, `Deps`, local `Config` projection, `DefaultEndpoint`.
- `login.go` / `logout.go` — write/clear the bearer token on the active profile.
- `whoami.go` — parallel `GetMember` + `GetOrganization` under a shared cancellable ctx; errors aggregated with `errors.Join`.
- `keys.go` — nested `auth keys create/rotate/revoke/list`. Plaintext is returned once via `apiKeyHash`.
- `service_accounts.go` — nested `auth service-accounts create/delete/list`.
- `errors.go` — `ErrNotLoggedIn` and `translateErr`, which classifies via the typed `IsAuthError()` interface and falls back to a `"unauthenticated"` string match.
- `*_test.go` — one per source file; shared fake `Deps` (mutex-guarded for whoami's fan-out) lives in `auth_test.go`.
