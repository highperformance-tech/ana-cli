# internal

All domain logic for the `ana` CLI. Each verb package is pure dispatch: it declares a narrow `Deps` struct, registers its Connect-RPC service prefix, and exposes a `New(deps) *cli.Group` (or a `cli.Command` leaf when there are no subcommands, as in `api/`) that `cmd/ana/main.go` wires up. Verb packages do not import `internal/transport` or `internal/config` (except `cli`, which is the dispatch core, and `profile`, whose whole purpose is config management).

## Test layout convention

Multi-file verb packages use one `<source>_test.go` per source file (e.g. `list.go` ↔ `list_test.go`). Shared test helpers (`fakeDeps`, `newIO`, etc.) plus package-surface tests (`TestNew*`, `TestHelp*`) live in `<pkg>_test.go`. A helper that is only used by one source file's tests travels with those tests; helpers used across multiple files stay in `<pkg>_test.go`.

## Children

| Path | What lives here |
|------|-----------------|
| `cli/` | Dispatch core: `Command` interface, `Group`, `Flagger`, `Resolve` + `Dispatch` (resolve-then-parse, Cobra-style), `Global`, exit-code mapping. |
| `testcli/` | Test helpers for verb packages (stdlib `httptest` analogue): `FailingWriter`, `FailingIO`, `NewIO`. |
| `config/` | Multi-profile config file reader/writer. XDG path resolution, `Resolve` precedence. |
| `transport/` | Connect-RPC HTTP client. Unary JSON + server-streaming JSON framing + `DoRaw` passthrough. Bearer + User-Agent applied via RoundTripper middleware. |
| `api/` | `ana api <path>` — raw authenticated HTTP passthrough for Connect-RPC short form + documented REST. Single leaf. |
| `auth/` | `ana auth` verb tree — login/logout/whoami/keys/service-accounts. |
| `profile/` | `ana profile` verb tree — add/use/remove/list/show. Imports `internal/config` by design. |
| `org/` | `ana org` — list/show + nested members/roles/permissions. |
| `chat/` | `ana chat` — CRUD + streaming `send` + share. |
| `connector/` | `ana connector` — CRUD + test/tables/examples. |
| `dashboard/` | `ana dashboard` — readonly list/get/folders/health + `spawn`. |
| `playbook/` | `ana playbook` — readonly list/get/reports/lineage. |
| `ontology/` | `ana ontology` — readonly list/get. |
| `feed/` | `ana feed` — show + stats. |
| `audit/` | `ana audit tail` — audit-log listing with `--since`. Injectable clock. |
| `update/` | Passive update-check nudge + `ana update` self-update verb. Stdlib-only; 100% covered. |
