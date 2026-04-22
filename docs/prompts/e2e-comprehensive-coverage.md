# Prompt: comprehensive e2e coverage for ana-cli

Paste this into a fresh Claude session inside the `ana-cli` repo
(`/Users/bfair/highperformance-tech/ana-cli`). The session will have no prior
conversation context, so read this carefully and investigate before acting.

---

## Goal

Bring `ana-cli`'s live smoke suite (`e2e/`) up to **comprehensive coverage of
every user-visible command and every meaningful argument variation** across
the entire CLI — not just the dialect-specific gaps we've been filling piecewise.
The existing suite was seeded opportunistically (postgres create, chat send,
auth round-trip, etc.) and we now need it to be a rigorous contract check.

A Snowflake-only starter PR exists on branch
`feature/e2e-snowflake-coverage` (1 commit, not yet pushed) that covers only
the four Snowflake `connector create` leaves. **Use that branch as your base
and build on it** — don't discard its harness additions (`RunStdin`,
`RegisterConnectorCleanup`, endpoint wiring in `e2e/harness/client.go`). Extend
them as needed.

## What "comprehensive" means here

For every verb package under `internal/` that the CLI exposes, every leaf
command must be smoked at least once with live RPC, plus meaningful argument
permutations (not a combinatorial explosion — the permutations that would
exercise distinct wire shapes or error paths). Specifically:

1. **Every leaf gets at least one happy-path test** that drives it through
   the CLI (`h.Run`/`h.RunStdin`), parses real stdout, asserts the created
   resource round-trips (get after create, get after update, etc.), and
   registers cleanup with the harness ledger.

2. **Per-leaf argument matrix** — read the leaf source to enumerate its flags.
   For each flag that changes wire shape (not just cosmetic flags like
   `--json`), add a test case that exercises that variation. Examples:
   - `--ssl=true` vs `--ssl=false` on postgres create
   - `--warehouse` present vs absent on snowflake create (omitempty path)
   - `--json` vs default output on every read leaf
   - partial updates on `connector update` (one field at a time) vs full
     merges (multiple fields in one call)
   - `--since <duration>` variants on `audit tail`
   - pagination / filter flags on every `list` verb that has them

3. **Stdin-capable secret flags** must be tested via the `--*-stdin`
   variant, not the inline `--password`/`--oauth-client-secret`/`--pat`
   variant. The in-args variants are fine to smoke once per leaf, but
   the primary path is stdin. Use `h.RunStdin(stdin, args...)`.

4. **Error-path smokes** for the obvious contract violations — e.g.,
   `connector get <nonexistent-id>` should fail with a typed RPC error,
   `connector create postgres password` missing `--user` should exit with
   `cli.ErrUsage`. Don't go chasing every possible error; hit the ones
   the server or dispatch enforces as a contract.

5. **Read-leaf coverage** — `list`, `get`, `tables`, `examples`, audit
   `tail`, feed `show`/`stats`, etc. — should cover both default output and
   `--json` output, asserting that the JSON is parseable and contains the
   fields the catalog (`api-catalog/`) says it should.

6. **Mutations require ledger discipline** — every `create` or `update` must
   register a cleanup with the harness so `sweepConnectors` (and the
   analogous sweeps for chats, profiles, service accounts, API keys)
   proves nothing leaked.

## Packages to cover

Work top-down through `internal/`, one verb package per commit (or small
logical group). Existing e2e files to reference:

| Package | Verbs | Existing e2e file |
|---------|-------|-------------------|
| `internal/auth` | login, logout, whoami, keys (list/create/rotate/revoke), service-accounts (list/create/delete) | `e2e/auth_test.go` |
| `internal/profile` | add, use, remove, list, show | — (none yet; profile is mostly local config, may need minimal live coverage) |
| `internal/org` | list, show, members list/get, roles list/get, permissions list/get | `e2e/org_test.go` |
| `internal/connector` | list, get, create (postgres password + 4 snowflake modes), update, delete, test, tables, examples | `e2e/connector_test.go`, `e2e/connector_snowflake_test.go` (new) |
| `internal/chat` | list, get, create, delete, send (streaming), share | `e2e/chat_test.go` |
| `internal/dashboard` | list, get, folders, health, spawn | — (none yet) |
| `internal/playbook` | list, get, reports, lineage | — (none yet) |
| `internal/ontology` | list, get | — (none yet) |
| `internal/feed` | show, stats | — (none yet) |
| `internal/audit` | tail (with `--since`) | `e2e/audit_test.go` |

The packages without an existing e2e file are your biggest gap. Create
`e2e/<verb>_test.go` files for each, mirroring the style of
`connector_snowflake_test.go` (per-test skip via `t.Skip` when env is missing,
CLI-path via `h.Run`, ledger-backed cleanup).

## Harness gaps you'll likely hit

1. **Read-only `list`/`get` leaves don't need cleanup.** Don't over-engineer —
   `h.Run` is enough.

2. **Streaming send (`chat send`)** already has a test; extend it to cover
   `--json` mode and `--attach-connector <id>`.

3. **`dashboard spawn`** creates a dashboard from a template. Assert the spawned
   dashboard shows up in `dashboard list` and register cleanup.

4. **Profile commands are mostly local.** `profile add`/`profile use`/
   `profile show` operate on `$XDG_CONFIG_HOME/ana/config.json` (already
   redirected to a temp dir in `e2e/harness/harness.go`'s `Begin`). These
   need e2e tests that use the harness's temp config, not a live RPC, but
   still through `h.Run` to exercise the dispatch path.

5. **Harness doesn't expose a way to manipulate `$XDG_CONFIG_HOME` directly.**
   Profile tests may need a helper; add it to `e2e/harness/` rather than
   reaching into private state from the test file.

## Environment variables

The existing `ANA_E2E_ENDPOINT` / `ANA_E2E_TOKEN` / `ANA_E2E_EXPECT_ORG` are
required globally. Per-leaf env needed:

- **Postgres connector** — `ANA_E2E_PG_*` (already defined; see
  `e2e/connector_test.go` for the full list).
- **Snowflake connector** — `ANA_E2E_SF_*` (already defined in
  `e2e/README.md` on the feature branch).
- **Chat** — requires a working connector. Tests create a throwaway connector
  (postgres preferred since it's the cheapest to skip) for the chat to hang
  off of, or accept `ANA_E2E_CONNECTOR_ID` to reuse an existing one.
- **Dashboard / playbook / ontology** — these leaves are mostly read-only
  for the MVP. Spawn/share/report variants may need `ANA_E2E_DASHBOARD_ID`,
  `ANA_E2E_PLAYBOOK_ID`, etc., as skip gates for the leaves that mutate.

Document every new env var in `e2e/README.md` as you add it, grouped by verb.

## Design rules

1. **Drive through `h.Run` / `h.RunStdin`**, not raw RPC helpers. The existing
   `CreateConnector(suffix, ConnSpec)` in `e2e/harness/resources.go` is a
   legacy shortcut kept for chat tests; don't add more of those. The whole
   point of e2e is to smoke the CLI itself.

2. **Per-test skip on missing env**, not file-level skip. Match
   `e2e/connector_snowflake_test.go` — each test prints exactly which vars
   it needs so operators know what's gated.

3. **Assertions must be resilient to server drift**: assert the presence of
   contract fields (`connectorId`, `connectorType`, etc.), not exact values
   for fields that could change (timestamps, display names set by the
   server).

4. **Cleanup runs in LIFO via `h.Register(func)`.** Every mutation registers
   a cleanup before completing the test — even if the test fails after
   creating the resource, cleanup still runs.

5. **Never log secrets.** Env-var values passed via `RunStdin` are piped to
   the leaf; don't `t.Logf` them. The harness already suppresses secret
   logging in `dryRun` mode — preserve that.

6. **`t.Parallel()` where safe.** Tests that create + delete their own
   resources are usually safe. Tests that manipulate org-level state
   (members, roles) may not be.

7. **Don't modify code in `internal/...`** unless you find an actual bug
   that blocks coverage — that belongs in a separate PR per the repo's
   conventions. Coverage gaps in the CLI implementation are fair game to
   flag but not to fix in this PR.

## Investigation first

Before writing any tests, read in this order:

1. `CLAUDE.md` (root) and every `CLAUDE.md` under `internal/` — these
   describe the verb-package conventions.
2. `e2e/CLAUDE.md` and `e2e/harness/CLAUDE.md` — the harness contract.
3. `e2e/harness/harness.go`, `e2e/harness/guard.go`, `e2e/harness/ledger.go`,
   `e2e/harness/sweep.go` — the mutation-safety plumbing.
4. Existing e2e tests (`e2e/auth_test.go`, `e2e/chat_test.go`,
   `e2e/connector_test.go`, `e2e/audit_test.go`, `e2e/org_test.go`,
   `e2e/connector_snowflake_test.go`) — the established style.
5. `api-catalog/` — wire-shape contracts for every endpoint.
6. Every leaf's `.go` source under `internal/<verb>/` — to enumerate flags.

## Scope of this PR

Ambitious but bounded:

- **In:** every `internal/<verb>/` package listed above, every leaf,
  meaningful argument variations, new env vars documented, updated
  `e2e/README.md` and `e2e/CLAUDE.md`.
- **Out:** changes to `internal/...` (CLI code), new `api-catalog/` entries,
  `Databricks` connector create tests (Databricks leaves haven't shipped
  yet — tracked as task #20), Phase 3g `update auth` subcommand (deferred).

If you find a gap in the CLI itself that blocks a test, **don't fix it
here** — note it in the PR description and open a separate issue.

## Workflow

1. Start on `feature/e2e-snowflake-coverage` (already exists, one commit
   ahead of main, not pushed). Don't rebase or force-push it.
2. Rename the branch to something broader — e.g.
   `feature/e2e-comprehensive-coverage` — via `git branch -m`.
3. Work one verb package per commit. Conventional commit messages:
   `feat(e2e): <verb> — <short summary>`.
4. After each package commit, run `make lint` and `make test` (not
   `make e2e` — that's live and requires env). Both must stay green.
5. `make cover` must keep showing 100% on `./internal/...` — this PR
   shouldn't touch `internal/` code, so this should be automatic.
6. **Dry-run each new test** via `ANA_E2E_DRYRUN=1 go test ./e2e/...`
   to confirm argv construction is correct without hitting the server.
7. When all packages are covered, **do not push or open a PR** — hand
   the branch back for human review and live smoke first.

## What to hand back when done

A single message summarizing:

- Which verb packages now have e2e coverage (one line each with the
  commit hash).
- How many new tests landed in total.
- Which flags you explicitly chose NOT to cover and why (there will be
  some — document the reasoning).
- Any CLI bugs or gaps you found while investigating (DO NOT fix them;
  note them for a follow-up PR).
- Confirmation that `make lint`, `make test`, and
  `ANA_E2E_DRYRUN=1 go test ./e2e/...` all pass.
- The exact `gh pr create` command you'd run (don't run it — the human
  will, after a live-smoke pass).
