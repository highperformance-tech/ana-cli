# ana e2e live smoke suite

Opt-in integration tests that drive the real TextQL API against a dedicated
demo org. Unit tests (under `internal/...`) run against `httptest.Server` fakes
and catch structural bugs. This suite catches **upstream drift** — a renamed
field, a changed status code, a reshuffled endpoint — by exercising every
captured verb end-to-end against a live org.

## Guarantees

- **State invariant**: the target org is identical at test-end as at
  test-start. Every mutation is either scoped to a disposable parent (Tier 1)
  or snapshot+restored (Tier 2). Unrecoverable mutations are refused up-front
  or written to a ledger file that fails the suite loudly.
- **Prefix**: every resource the suite creates is named
  `anacli-e2e-<unix>-<hex>…` so a pre-run sweep can reap leftovers from prior
  crashed runs.
- **Org guard**: before any RPC the harness calls `GetOrganization` and
  aborts if the resolved `orgId` does not match `ANA_E2E_EXPECT_ORG_ID`. The
  id (not the display name) is checked so renaming the tenant can't silently
  widen the blast radius.

## Running

Required env:

| Variable                | Meaning                                                |
|-------------------------|--------------------------------------------------------|
| `ANA_E2E_ENDPOINT`      | Base URL, usually `https://app.textql.com`             |
| `ANA_E2E_TOKEN`         | API key for the dedicated demo account                 |
| `ANA_E2E_EXPECT_ORG_ID` | UUID `orgId` the token must resolve to                 |

Optional env:

| Variable               | Effect                                             |
|------------------------|----------------------------------------------------|
| `ANA_E2E_DRYRUN=1`     | Log planned mutations without issuing RPCs         |
| `ANA_E2E_SWEEP_ONLY=1` | Run the leftover-sweep only, then skip tests       |
| `ANA_E2E_DASHBOARD_ID` | Existing dashboard id to probe (`health`, `spawn`); unset skips those leaves |

### Postgres connector env

Connector tests default to a syntactically valid but unreachable spec
(`host=e2e.invalid`); `CreateConnector` accepts it because only `connector
test` actually dials the db. Set these to exercise the driver path against a
reachable Postgres:

| Variable               | Meaning                                           |
|------------------------|---------------------------------------------------|
| `ANA_E2E_PG_HOST`      | Hostname (default `e2e.invalid`; required for `connector tables`) |
| `ANA_E2E_PG_PORT`      | TCP port (default `5432`)                         |
| `ANA_E2E_PG_USER`      | Username (default `e2e`)                          |
| `ANA_E2E_PG_PASSWORD`  | Password (default `e2e`); piped via `--password-stdin` |
| `ANA_E2E_PG_DATABASE`  | Database name (default `postgres`)                |

### Snowflake connector env

Snowflake tests (`e2e/connector_snowflake_test.go`) skip per-test when their
required vars are absent — unlike Postgres, there are no sensible defaults.
Two vars are shared across every mode; set both or every Snowflake test
skips:

| Variable               | Meaning                                                         |
|------------------------|-----------------------------------------------------------------|
| `ANA_E2E_SF_LOCATOR`   | Snowflake account locator (e.g. `abc12345.us-east-1`)           |
| `ANA_E2E_SF_DATABASE`  | Database name (required)                                        |
| `ANA_E2E_SF_WAREHOUSE` | Default warehouse (optional; unset to exercise omitempty)       |
| `ANA_E2E_SF_SCHEMA`    | Default schema (optional)                                       |
| `ANA_E2E_SF_ROLE`      | Default role (optional)                                         |

Password mode (`TestConnectorCreateSnowflakePassword`):

| Variable               | Meaning                                                         |
|------------------------|-----------------------------------------------------------------|
| `ANA_E2E_SF_USER`      | Snowflake username                                              |
| `ANA_E2E_SF_PASSWORD`  | Password; piped via `--password-stdin`                          |

Keypair mode (`TestConnectorCreateSnowflakeKeypair`):

| Variable                             | Meaning                                           |
|--------------------------------------|---------------------------------------------------|
| `ANA_E2E_SF_USER`                    | Snowflake username bound to the public key        |
| `ANA_E2E_SF_PRIVATE_KEY_PATH`        | Path to a PEM-encoded PKCS#8 private key file     |
| `ANA_E2E_SF_PRIVATE_KEY_PASSPHRASE`  | Optional; piped via `--private-key-passphrase-stdin` when set |

OAuth SSO + OAuth individual (`TestConnectorCreateSnowflakeOAuthSSO`,
`TestConnectorCreateSnowflakeOAuthIndividual`) share the same vars and only
differ in wire `authStrategy`:

| Variable                          | Meaning                                                 |
|-----------------------------------|---------------------------------------------------------|
| `ANA_E2E_SF_OAUTH_CLIENT_ID`      | Snowflake OAuth client id                               |
| `ANA_E2E_SF_OAUTH_CLIENT_SECRET`  | Client secret; piped via `--oauth-client-secret-stdin`  |

### Databricks connector env

Databricks tests (`e2e/connector_databricks_test.go`) skip per-test when
their required vars are absent — same treatment as Snowflake. Four vars are
shared across every mode; set all four or every Databricks test skips:

| Variable                | Meaning                                                        |
|-------------------------|----------------------------------------------------------------|
| `ANA_E2E_DBX_HOST`      | Workspace hostname without scheme (e.g. `dbc-xxxx.cloud.databricks.com`) |
| `ANA_E2E_DBX_HTTP_PATH` | SQL warehouse path (`/sql/1.0/warehouses/<id>`)               |
| `ANA_E2E_DBX_CATALOG`   | Unity Catalog name                                            |
| `ANA_E2E_DBX_SCHEMA`    | Default schema                                                |
| `ANA_E2E_DBX_PORT`      | Optional port override (defaults to 443 when unset)           |

Access Token mode (`TestConnectorCreateDatabricksAccessToken`):

| Variable              | Meaning                                                        |
|-----------------------|----------------------------------------------------------------|
| `ANA_E2E_DBX_TOKEN`   | Personal Access Token; piped via `--token-stdin`              |

Client Credentials mode (`TestConnectorCreateDatabricksClientCredentials`):

| Variable                    | Meaning                                                        |
|-----------------------------|----------------------------------------------------------------|
| `ANA_E2E_DBX_CLIENT_ID`     | Service Principal OAuth applicationId (UUID)                  |
| `ANA_E2E_DBX_CLIENT_SECRET` | Service Principal OAuth secret; piped via `--client-secret-stdin` |

OAuth SSO + OAuth individual (`TestConnectorCreateDatabricksOAuthSSO`,
`TestConnectorCreateDatabricksOAuthIndividual`) share the same vars — the
Databricks OAuth app credentials, distinct from the Service Principal
credentials used by Client Credentials mode:

| Variable                          | Meaning                                                 |
|-----------------------------------|---------------------------------------------------------|
| `ANA_E2E_DBX_OAUTH_CLIENT_ID`     | Databricks OAuth app client id                          |
| `ANA_E2E_DBX_OAUTH_CLIENT_SECRET` | OAuth app secret; piped via `--client-secret-stdin`     |

Invocations:

```sh
export ANA_E2E_ENDPOINT=https://app.textql.com
export ANA_E2E_TOKEN=<demo-key>
export ANA_E2E_EXPECT_ORG_ID=<demo-org-uuid>

make e2e-dryrun   # list every planned mutation
make e2e-sweep    # clean stale anacli-e2e-* leftovers
make e2e          # full run
```

With none of the three required env vars set, `go test ./...` skips the suite
entirely (via `t.Skip`), so CI that does not provide credentials stays green.

## Tiers

- **Tier 1 — create-scoped**: test creates the resource, defer-deletes. Every
  nested mutation against a test-created id cascades on parent delete. Default
  tier — covers almost every captured verb.
- **Tier 2 — mutate-restore**: snapshot, mutate, restore on cleanup. Only used
  when the mutation target is a singleton (e.g. the org itself).
- **Tier 3 — manual-revert**: escape hatch. Currently empty. New captured
  verbs whose reverse is unknown call `h.RecordManualRevert(...)`; the
  harness flushes `e2e-manual-revert-<ts>.md` and fails the suite so a human
  reviews the ledger before state accumulates.

## Ledger

If any cleanup fails or any test records a manual-revert entry, the harness
writes `e2e-manual-revert-<ts>.md` (gitignored) next to the temp config and
`t.Fatalf`s the suite. Review the checklist, revert by hand, and re-run.

## Architecture

- `harness/harness.go` — `Begin` / `End`, cleanup registry, guard, ledger
- `harness/client.go`  — duplicates `cmd/ana/main.go` wiring so `h.Run(...)`
  exercises the full dispatch path, not just transport
- `harness/resources.go` — Tier-1 create helpers (`CreateConnector`, etc.)
- `harness/snapshot.go` — Tier-2 snapshot/restore helpers
- `harness/sweep.go`   — leftover-sweep called from `Begin`
- `harness/ledger.go`  — manual-revert ledger + flush logic
- `harness/guard.go`   — `ANA_E2E_EXPECT_ORG_ID` check
- `*_test.go`          — per-verb test files (one file per surface area)
