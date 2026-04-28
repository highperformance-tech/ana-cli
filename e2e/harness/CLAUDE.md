# e2e/harness

Per-test scaffolding for live smoke tests against a real TextQL endpoint. Duplicates the `cmd/ana/main.go` verb-wiring so tests can drive CLI verbs and raw RPCs through the same transport. Every mutation is guarded: the harness tracks it, reverts in LIFO order at `End`, and writes anything it can't revert to a `manual-revert.md` report for operator follow-up.

## Files

- `harness.go` — `H` + `Begin`/`End`. Per-test lifecycle (temp config, auth env, verb map, cleanup stack). Exposes `ExpectOrgID()` / `Endpoint()` for stdout assertions.
- `client.go` — mirrors `cmd/ana/main.go`'s verb builder so harness and binary share one wiring shape.
- `guard.go` — wraps mutating RPCs: ledger-record before invoke, abort on pre-flight failure (wrong org, missing env, etc.).
- `ledger.go` — `ManualRevertLog` + `Record`/`Close`; renders unreverted mutations via the testdata template.
- `resources.go` — factories for throwaway connectors/chats/profiles; each factory registers its own revert. Also: name-based safety-net cleanups (`RegisterConnectorCleanupByName`, `RegisterAPIKeyCleanupByName`, `RegisterServiceAccountCleanupByName`) for the gap between create and id-extraction.
- `snapshot.go`, `sweep.go` — pre-test state capture + post-test diff so anything new that slipped past the ledger fails the test.
