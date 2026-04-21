# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `types.go` — shared wire shapes consumed by create + update: `createReq`, `updateReq`, `configEnvelope` (carries `authStrategy` at envelope level — not nested under a dialect sub-object), `postgresSpec`, `snowflakeSpec` (one struct covers all four auth modes; server discriminates by populated field + envelope `authStrategy`), `createResp`, `getConnectorResp`.
- `list.go` / `get.go` — `GetConnectors` / `GetConnector` (readonly).
- `create.go` — `newCreateGroup` (dialect-selector Group) + the shared `resolvePassword` helper reused by per-dialect password leaves and `update.go`.
- `create_postgres.go` — `newPostgresCreateGroup` (Postgres dialect Group whose inheritable `Flags` closure owns `--name`/`--ssl`) and `postgresPasswordCmd` (leaf for the `password` auth mode). Implements `cli.Flagger` so `--help` enumerates own + ancestor flags. Additional Postgres auth modes (key-based, cert-based) land as sibling leaf files — no reshuffling.
- `create_snowflake.go` — `newSnowflakeCreateGroup` (Snowflake dialect Group whose inheritable `Flags` closure owns `--name`/`--locator`/`--database`/`--warehouse`/`--schema`/`--role`). `locator` is TextQL's wire name for Snowflake's `account`. Sibling auth-mode leaves (`password`, and the forthcoming `keypair`/`oauth-sso`/`oauth-individual`) live in `create_snowflake_<mode>.go`.
- `create_snowflake_password.go` — `snowflakePasswordCmd` (leaf for `snowflake password`). Declares `--user` + `--password{,-stdin}`; sets envelope `authStrategy=service_role` and populates `snowflakeSpec.{Username,Password}`.
- `update.go` — `UpdateConnector`. Pre-fetches the baseline so interleaved flags merge correctly (see commit `1433e01`). Still a flat leaf today; dialect-aware validation + auth-mode-swap subtree are Phase 3g work.
- `delete.go`, `test.go` (TestConnector), `tables.go` (ListConnectorTables), `examples.go` (GetExampleQueries) — remaining CRUD + diagnostic verbs.
- `connector_test.go` — shared `fakeDeps`, `errReader`, `TestNew*`/`TestHelp*`.
- `create_postgres_password_test.go` — covers the Postgres password leaf end-to-end by dispatching through `newCreateGroup` so ancestor-flag plumbing (`--name`, `--ssl` from the Postgres Group) is exercised the same way real CLI dispatch does. `requiredArgs()` builder starts with `"postgres", "password"` because every test routes through the Group.
- `create_snowflake_password_test.go` — same shape as the Postgres test, routed through `newCreateGroup`. Asserts envelope `authStrategy=service_role`, populated `username`/`password`, and that unset optional fields (`warehouse`/`schema`/`role`) are omitted via `omitempty`.
- `list_test.go` / `get_test.go` / `update_test.go` / `delete_test.go` / `test_test.go` / `tables_test.go` / `examples_test.go` — one per non-create source file.
