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
  aborts if the resolved `organizationName` does not match
  `ANA_E2E_EXPECT_ORG`.

## Running

Required env:

| Variable             | Meaning                                                   |
|----------------------|-----------------------------------------------------------|
| `ANA_E2E_ENDPOINT`   | Base URL, usually `https://app.textql.com`                |
| `ANA_E2E_TOKEN`      | API key for the dedicated demo account                    |
| `ANA_E2E_EXPECT_ORG` | Human-readable org name the token must resolve to         |

Optional env:

| Variable               | Effect                                             |
|------------------------|----------------------------------------------------|
| `ANA_E2E_DRYRUN=1`     | Log planned mutations without issuing RPCs         |
| `ANA_E2E_SWEEP_ONLY=1` | Run the leftover-sweep only, then skip tests       |
| `ANA_E2E_PG_HOST` etc. | Use a real postgres for connector tests (optional) |

Invocations:

```sh
export ANA_E2E_ENDPOINT=https://app.textql.com
export ANA_E2E_TOKEN=<demo-key>
export ANA_E2E_EXPECT_ORG="Example Org"

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
- `harness/guard.go`   — `ANA_E2E_EXPECT_ORG` check
- `*_test.go`          — per-verb test files (one file per surface area)
