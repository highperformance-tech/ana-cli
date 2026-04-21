# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `types.go` — shared wire shapes for create + update.
- `list.go` / `get.go` — readonly `GetConnectors` / `GetConnector`.
- `create.go` — dialect-selector Group and the shared `resolveSecret` helper.
- `create_postgres.go` — Postgres dialect Group; sibling files add auth-mode leaves.
- `create_snowflake.go` — Snowflake dialect Group; sibling files add auth-mode leaves.
- `create_snowflake_password.go` — `snowflake password` leaf.
- `create_snowflake_keypair.go` — `snowflake keypair` leaf; reads PEM key from file.
- `create_snowflake_oauth_sso.go` — `snowflake oauth-sso` leaf.
- `create_snowflake_oauth_individual.go` — `snowflake oauth-individual` leaf.
- `update.go` — `UpdateConnector`; pre-fetches baseline to merge partial updates.
- `delete.go`, `test.go`, `tables.go`, `examples.go` — remaining CRUD + diagnostic verbs.
- `connector_test.go` — shared `fakeDeps`, `errReader`, `TestNew*`/`TestHelp*`.
- `create_postgres_password_test.go` — covers the Postgres password leaf via `newCreateGroup`.
- `create_snowflake_password_test.go` — covers the Snowflake password leaf via `newCreateGroup`.
- `create_snowflake_keypair_test.go` — covers the keypair leaf including key-file edge cases.
- `create_snowflake_oauth_sso_test.go` — covers the oauth-sso leaf.
- `create_snowflake_oauth_individual_test.go` — covers the oauth-individual leaf.
- `list_test.go` / `get_test.go` / `update_test.go` / `delete_test.go` / `test_test.go` / `tables_test.go` / `examples_test.go` — one per non-create source file.
