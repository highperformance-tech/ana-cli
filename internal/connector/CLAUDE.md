# internal/connector

The `ana connector` verb tree: `list`, `get`, `create`, `update`, `delete`, `test`, `tables`, `examples`. Dispatch-only around `Deps.Unary`.

## Files

- `connector.go` — `New`, `Deps`, service path prefix.
- `types.go` — shared wire shapes for create + update.
- `types_postgres.go` — Postgres wire spec.
- `types_snowflake.go` — Snowflake wire spec.
- `types_databricks.go` — Databricks wire spec: `databricksSpec` + nested `databricksAuth` one-of (`pat`/`clientCredentials`/`oauthU2m`).
- `list.go` / `get.go` — readonly `GetConnectors` / `GetConnector`.
- `create.go` — dialect-selector Group and the shared `resolveSecret` helper.
- `create_postgres.go` — Postgres dialect Group; sibling files add auth-mode leaves.
- `create_snowflake.go` — Snowflake dialect Group; sibling files add auth-mode leaves.
- `create_snowflake_password.go` — `snowflake password` leaf.
- `create_snowflake_keypair.go` — `snowflake keypair` leaf; reads PEM key from file.
- `create_snowflake_oauth_sso.go` — `snowflake oauth-sso` leaf.
- `create_snowflake_oauth_individual.go` — `snowflake oauth-individual` leaf.
- `create_databricks.go` — Databricks dialect Group + shared `requireDatabricksCommon` validator; sibling files add auth-mode leaves.
- `create_databricks_access_token.go` — `databricks access-token` leaf (PAT → `databricksAuth.pat`).
- `create_databricks_client_credentials.go` — `databricks client-credentials` leaf (M2M → `databricksAuth.clientCredentials`).
- `create_databricks_oauth_sso.go` — `databricks oauth-sso` leaf (U2M → `databricksAuth.oauthU2m`, `authStrategy=oauth_sso`).
- `create_databricks_oauth_individual.go` — `databricks oauth-individual` leaf (same `oauthU2m` variant, `authStrategy=per_member_oauth`).
- `update.go` — `UpdateConnector`; pre-fetches baseline to merge partial updates.
- `delete.go`, `test.go`, `tables.go`, `examples.go` — remaining CRUD + diagnostic verbs.
- `connector_test.go` — shared `fakeDeps`, `errReader`, `TestNew*`/`TestHelp*`.
- `create_postgres_password_test.go` — covers the Postgres password leaf via `newCreateGroup`.
- `create_snowflake_password_test.go` — covers the Snowflake password leaf via `newCreateGroup`.
- `create_snowflake_keypair_test.go` — covers the keypair leaf including key-file edge cases.
- `create_snowflake_oauth_sso_test.go` — covers the oauth-sso leaf.
- `create_snowflake_oauth_individual_test.go` — covers the oauth-individual leaf.
- `create_databricks_access_token_test.go` / `create_databricks_client_credentials_test.go` / `create_databricks_oauth_sso_test.go` / `create_databricks_oauth_individual_test.go` — one per Databricks leaf; same structure as the Snowflake equivalents.
- `list_test.go` / `get_test.go` / `update_test.go` / `delete_test.go` / `test_test.go` / `tables_test.go` / `examples_test.go` — one per non-create source file.
