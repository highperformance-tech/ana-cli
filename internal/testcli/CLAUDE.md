# internal/testcli

Test scaffolding for verb-package unit tests. Mirrors the stdlib split (`net/http/httptest`, `testing/iotest`) — production code stays in `internal/cli` and test-only helpers live here, so the cli package itself carries no test types. Non-`_test.go` file is required because consumers live in other packages; dead-code eliminated from the production binary since no non-test import reaches it.

## Files

- `testcli.go` — `FailingWriter` (always-erroring `io.Writer` for exercising write/flush error branches), `FailingIO` (`cli.IO` whose Stdout fails; shares the same fixed-epoch `Now` as `NewIO` so both constructors are deterministic), `NewIO(stdin)` (`cli.IO` wired to in-memory buffers with fixed-epoch `Now` for deterministic assertions; `nil` stdin defaults to an empty reader).
- `testcli_test.go` — exercises every branch for 100% coverage.
