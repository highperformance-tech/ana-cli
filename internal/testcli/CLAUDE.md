# internal/testcli

Test scaffolding for verb-package unit tests. Mirrors the stdlib split (`net/http/httptest`, `testing/iotest`) — production code stays in `internal/cli` and test-only helpers live here, so the cli package itself carries no test types. Non-`_test.go` file is required because consumers live in other packages; dead-code eliminated from the production binary since no non-test import reaches it.

## Files

- `testcli.go` — `FailingWriter`, `FailingIO`, `NewIO` (in-memory streams + fixed-epoch `Now`), and `RecordUnary` (Unary closure that captures path + JSON request before delegating, collapsing the per-package `fakeDeps.Unary` duplication).
- `testcli_test.go` — 100% coverage gate.
