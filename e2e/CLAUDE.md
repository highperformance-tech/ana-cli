# e2e

Live-smoke tests that drive real `app.textql.com` RPCs through the same verb packages the binary uses. Opt-in: set `ANA_E2E_ENDPOINT` + `ANA_E2E_TOKEN` (and friends — see `README.md`) and run `make e2e`. Every mutation is ledgered and reverted in LIFO order at test end; anything the harness cannot revert lands in a `manual-revert.md` report for operator follow-up.

## Children

| Path | What lives here |
|------|-----------------|
| `README.md` | Setup (env vars, `.env`), how to run a single suite, safety rails. |
| `harness/` | `H`, `Begin`/`End`, guarded mutations, resource factories, pre/post snapshot sweep. |
| `testdata/` | Static fixtures — currently just the `manual-revert.md` template. |
| `audit_test.go` | Audit-log `tail` smoke. |
| `auth_test.go` | Keys + service-accounts CRUD via CLI; `--json` shape + usage-guard error paths. |
| `chat_test.go` | Chat CRUD + streaming `send`; CLI-path `new`/`history`/`bookmark`/`unbookmark` and `show <missing>` error path. |
| `connector_test.go` | Connector CRUD + `--json`; postgres create matrix; `tables`/`examples`/`test`; `get <missing>` error path. |
| `connector_create_leaves_test.go` | Dialect-neutral `connectorCreateLeaf` helper; every Snowflake/Databricks create leaf round-trips through it so the pattern can't drift. |
| `connector_snowflake_test.go`, `connector_databricks_test.go` | Per-dialect create-leaf smokes; per-auth-mode env-gated. |
| `dashboard_test.go` | List/get/folders default + `--json`; `health`/`spawn` env-gated on `ANA_E2E_DASHBOARD_ID`. |
| `playbook_test.go` | List/get/reports/lineage default + `--json`; id discovered via `list --json`. |
| `ontology_test.go` | List/get default + `--json`; integer id on the wire. |
| `feed_test.go` | Show + stats (default + `--json`). |
| `profile_test.go` | Add/list/use/show/remove round-trip through a harness temp XDG; missing-name + unknown-profile error paths. |
| `org_test.go` | List/show + nested members/roles/permissions; `--json` shape assertions. |
