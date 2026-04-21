# e2e

Live-smoke tests that drive real `app.textql.com` RPCs through the same verb packages the binary uses. Opt-in: set `ANA_E2E_ENDPOINT` + `ANA_E2E_TOKEN` (and friends — see `README.md`) and run `make e2e`. Every mutation is ledgered and reverted in LIFO order at test end; anything the harness cannot revert lands in a `manual-revert.md` report for operator follow-up.

## Children

| Path | What lives here |
|------|-----------------|
| `README.md` | Setup (env vars, `.env`), how to run a single suite, safety rails. |
| `harness/` | `H`, `Begin`/`End`, guarded mutations, resource factories, pre/post snapshot sweep. |
| `testdata/` | Static fixtures — currently just the `manual-revert.md` template. |
| `audit_test.go` | Audit-log `tail` smoke. |
| `auth_test.go` | Login/logout/whoami round-trip against a real org. |
| `chat_test.go` | Chat CRUD + streaming `send`. |
| `connector_test.go` | Connector CRUD (create/update/test/delete) with ledger-backed cleanup. |
| `connector_snowflake_test.go` | Snowflake create leaves (password/keypair/oauth-sso/oauth-individual), per-mode env-gated. |
| `dashboard_test.go` | Dashboard list/get/folders read leaves (default + `--json`); `health`/`spawn` env-gated on `ANA_E2E_DASHBOARD_ID`. |
| `org_test.go` | Org list/show + nested members/roles/permissions. |
