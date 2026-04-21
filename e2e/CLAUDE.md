# e2e

Live-smoke tests that drive real `app.textql.com` RPCs through the same verb packages the binary uses. Opt-in: set `ANA_E2E_ENDPOINT` + `ANA_E2E_TOKEN` (and friends — see `README.md`) and run `make e2e`. Every mutation is ledgered and reverted in LIFO order at test end; anything the harness cannot revert lands in a `manual-revert.md` report for operator follow-up.

## Children

| Path | What lives here |
|------|-----------------|
| `README.md` | Setup (env vars, `.env`), how to run a single suite, safety rails. |
| `harness/` | `H`, `Begin`/`End`, guarded mutations, resource factories, pre/post snapshot sweep. |
| `testdata/` | Static fixtures — currently just the `manual-revert.md` template. |
| `audit_test.go` | Audit-log `tail` smoke. |
| `auth_test.go` | Keys + service-accounts CLI-driven create/rotate/revoke/delete (helper-backed legacy tests still live here for coverage); `--json` shape checks + error-path smokes for usage guards. |
| `chat_test.go` | Chat CRUD + streaming `send`. |
| `connector_test.go` | Connector CRUD (create/update/test/delete) with ledger-backed cleanup. |
| `connector_snowflake_test.go` | Snowflake create leaves (password/keypair/oauth-sso/oauth-individual), per-mode env-gated. |
| `dashboard_test.go` | Dashboard list/get/folders read leaves (default + `--json`); `health`/`spawn` env-gated on `ANA_E2E_DASHBOARD_ID`. |
| `playbook_test.go` | Playbook list/get/reports/lineage read leaves (default + `--json`); id discovered via `list --json`. |
| `ontology_test.go` | Ontology list/get read leaves (default + `--json`); id is integer on the wire. |
| `feed_test.go` | Feed show + stats (default + `--json`). |
| `profile_test.go` | Profile add/list/use/show/remove round-trip through harness temp XDG; error-path smokes for missing name + unknown profile. |
| `org_test.go` | Org list/show + nested members/roles/permissions, each with `--json` shape assertions. |
