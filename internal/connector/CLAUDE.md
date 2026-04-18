# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `list.go` / `get.go` — `GetConnectors` / `GetConnector` (readonly).
- `create.go` — `CreateConnector`. Postgres dialect verified; other dialects assumed from captured samples.
- `update.go` — `UpdateConnector`. Pre-fetches the baseline so interleaved flags merge correctly (see commit `1433e01`).
- `delete.go`, `test.go` (TestConnector), `tables.go` (ListConnectorTables), `examples.go` (GetExampleQueries) — remaining CRUD + diagnostic verbs.
- `flags.go` — shared flag registration (credentials, dialect, connector id).
- `connector_test.go` — fake `Unary` covers every subcommand + the update-baseline-merge path.
