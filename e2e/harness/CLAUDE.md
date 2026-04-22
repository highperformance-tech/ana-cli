# e2e/harness

Per-test scaffolding for live smoke tests against a real TextQL endpoint. Duplicates the `cmd/ana/main.go` verb-wiring so tests can drive CLI verbs and raw RPCs through the same transport. Every mutation is guarded: the harness tracks it, reverts in LIFO order at `End`, and writes anything it can't revert to a `manual-revert.md` report for operator follow-up.

## Files

- `harness.go` — `H`, `Begin`, `End`. Per-test lifecycle with temp config, auth env, verb map, and cleanup stack.
- `client.go` — mirrors `cmd/ana/main.go`'s verb builder so harness and binary share the same wiring shape.
- `guard.go` — wraps mutating RPCs: records them on the ledger before invoking, aborts if the pre-flight guard fails (wrong org, missing env, etc.).
- `ledger.go` — `ManualRevertLog` + `Record`/`Close`. Writes any unreverted mutation using `e2e/testdata/manual-revert.template.md`.
- `resources.go` — factories for throwaway connectors, chats, profiles, etc. Every factory registers its own revert with the harness; CLI-driven tests can also pre-register name-based safety-net cleanups (`RegisterConnectorCleanupByName`, `RegisterAPIKeyCleanupByName`, `RegisterServiceAccountCleanupByName`) for the gap between create and id-extraction.
- `snapshot.go` — captures the pre-test state of the target org (connector list, etc.) so `sweep.go` can prove nothing leaked.
- `sweep.go` — post-test diff of snapshot vs. current state; fails the test if anything new slipped past the ledger.
