# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `types.go`, `types_postgres.go`, `types_snowflake.go`, `types_databricks.go` — shared + per-dialect wire shapes. Databricks uses a nested `databricksAuth` one-of (`pat`/`clientCredentials`/`oauthU2m`).
- `list.go`, `get.go` — `GetConnectors` / `GetConnector` (readonly).
- `create.go` — dialect-selector Group + shared `resolveSecret`.
- `create_postgres.go`, `create_snowflake.go`, `create_databricks.go` — per-dialect Groups (Databricks adds the shared `requireDatabricksCommon` validator); the per-auth-mode leaves live in sibling `create_<dialect>_<mode>.go` files (snowflake: password / keypair / oauth-sso / oauth-individual; databricks: access-token / client-credentials / oauth-sso / oauth-individual).
- `update.go` — `UpdateConnector`; pre-fetches baseline so partial updates merge cleanly.
- `delete.go`, `test.go`, `tables.go`, `examples.go` — remaining CRUD + diagnostic verbs.
- `*_test.go` — one per source; shared fakes in `connector_test.go`.
