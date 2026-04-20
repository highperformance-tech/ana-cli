# internal/auth

The `ana auth` verb tree: `login`, `logout`, `whoami`, plus nested `keys` and `service-accounts` groups. Pure dispatch logic around an injected `Deps` struct (Unary RPC + config load/save closures) so the package never imports `internal/transport` or `internal/config` directly. Declares its own narrow `Config` (endpoint + `cli.Token`) to keep the contract narrow; the token field uses the redacting string type so accidental `%v`/`%s` formatting never leaks a bearer token.

## Files

- `auth.go` — `New`, `Deps`, local `Config` projection, `DefaultEndpoint`.
- `login.go` / `logout.go` — persist the bearer token to the active profile slot (login) or blank it out (logout).
- `whoami.go` — fans out `PublicAuthService.GetMember` and `GetOrganization` in parallel under a shared cancellable ctx so the first failure cancels the sibling RPC; errors are aggregated with `errors.Join` for a deterministic error surface.
- `keys.go` — nested group: `ana auth keys create/rotate/revoke/list`. `CreateApiKey` returns the plaintext once as `apiKeyHash`; the command prints it and exits.
- `service_accounts.go` — nested group: `ana auth service-accounts create/delete/list`.
- `errors.go` — `ErrNotLoggedIn` (`cli.ExitCode` maps to 2), plus the `authErr`/`Unwrap`/`IsAuthError` helpers and `translateErr`, which classifies auth failures via the typed `IsAuthError()` interface (the path `*transport.Error` takes) and falls back to a string match on `"unauthenticated"` for servers that surface only the message.
- `auth_test.go` / `login_test.go` / `logout_test.go` / `whoami_test.go` / `keys_test.go` / `service_accounts_test.go` / `errors_test.go` — one test file per source file, backed by a shared fake `Deps` (with a `sync.Mutex` for `whoami`'s concurrent fan-out) and `t.TempDir` configs. `auth_test.go` hosts only the shared helpers plus `TestNew*`/`TestHelp*`.
