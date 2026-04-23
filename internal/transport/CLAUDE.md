# internal/transport

Minimal Connect-RPC over HTTP client used by every verb package. Supports unary JSON calls and server-streaming JSON responses (5-byte Connect frame header: `[flags:1][length:4 BE][payload]`). No code-gen — request/response shapes are `any`, JSON-encoded/decoded by the caller.

## Files

- `client.go` — `Client`, `New`, functional `Option`s (`WithHTTPClient`, `WithUserAgent`), `Unary`, `Stream`, and `DoRaw` (raw authenticated HTTP used by `internal/api`). Injects a `tokenFn` so the transport stays agnostic to where the bearer token comes from. Bearer + User-Agent attach via a `bearerTransport` RoundTripper middleware wrapped around the configured `http.Client.Transport` — every call site (Unary, Stream, DoRaw) inherits auth for free; there is no per-call-site header plumbing.
- `stream.go` — `StreamReader` (one `Next`/`Close` per frame). Terminal frame has the `trailerFlag` bit set and either an empty body or a `{code, message}` error envelope.
- `error.go` — `Error` (wraps HTTP status + Connect error code/message), the `IsAuth` predicate used by commands to surface `auth.ErrNotLoggedIn`, and the `IsAuthError()` method that lets `*Error` satisfy the unexported `IsAuthError() bool` interface picked up by both `cli.ExitCode` and `auth.translateErr` — the typed escape hatch that replaces string-matching `"unauthenticated"`.
- `client_test.go`, `stream_test.go`, `error_test.go`, `transport_test.go` — drive `httptest.Server` instances to cover happy paths, mid-stream errors, trailer parsing, and auth classification.
