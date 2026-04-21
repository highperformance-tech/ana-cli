# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `types.go` — shared wire shapes consumed by create + update: `createReq`, `updateReq`, `configEnvelope`, `postgresSpec`, `createResp`, `getConnectorResp`. Per-dialect spec types will land alongside (`types_snowflake.go`, etc.) as new dialects are probed.
- `list.go` / `get.go` — `GetConnectors` / `GetConnector` (readonly).
- `create.go` — `CreateConnector`. Postgres dialect verified; other dialects assumed from captured samples.
- `update.go` — `UpdateConnector`. Pre-fetches the baseline so interleaved flags merge correctly (see commit `1433e01`).
- `delete.go`, `test.go` (TestConnector), `tables.go` (ListConnectorTables), `examples.go` (GetExampleQueries) — remaining CRUD + diagnostic verbs.
- `connector_test.go` — shared `fakeDeps`, `errReader`, `TestNew*`/`TestHelp*`.
- `list_test.go` / `get_test.go` / `create_test.go` / `update_test.go` / `delete_test.go` / `test_test.go` / `tables_test.go` / `examples_test.go` — one per source file. `create_test.go` carries the `requiredFlags()` builder used by the create cases; `update_test.go` covers the baseline-merge path via `cli.FlagWasSet`.
