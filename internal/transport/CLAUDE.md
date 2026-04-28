# internal/transport

Minimal Connect-RPC over HTTP client used by every verb package. Supports unary JSON calls and server-streaming JSON responses (5-byte Connect frame header: `[flags:1][length:4 BE][payload]`). No code-gen — request/response shapes are `any`, JSON-encoded/decoded by the caller.

## Files

- `client.go` — `Client`, `New`, `Option`s, `Unary`, `Stream`, `DoRaw`. Bearer + User-Agent attach via a RoundTripper middleware, so every call path inherits auth without per-site header plumbing.
- `stream.go` — `StreamReader` (`Next`/`Close` per frame). Terminal frame sets `trailerFlag` and may carry a `{code, message}` envelope.
- `error.go` — `Error` + the typed `IsAuthError()` interface that replaces string-matching `"unauthenticated"` (picked up by `cli.ExitCode` and `auth.translateErr`).
- `client_test.go`, `stream_test.go`, `error_test.go`, `transport_test.go` — `httptest.Server`-driven coverage; 100% gate.
