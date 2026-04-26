# cmd/ana

The `ana` binary's main package. Pure wiring: reads global flags + config, constructs a `transport.Client`, assembles the verb map by injecting adapted `Deps` into each `internal/<verb>` package, and hands off to `cli.Dispatch`. All domain logic lives under `internal/`.

## Files

- `main.go` — `main` + `run` (the testable entrypoint with injectable args/stdio/env), the root `*cli.Group` declaration whose persistent `Flags` closure binds `--json`/`--endpoint`/`--token-file`/`--profile` into a `cli.Global`, and `lazyState` + `buildVerbs`. `lazyState` defers config-load and transport-client construction until the first verb method that needs them, so `ana profile add` (which manages config itself) never touches the existing file or builds an HTTP client for an org it's about to create. Also owns `newUUID` (chat cellIds), the projection `profileToAuthConfig` that keeps `internal/auth` from importing `internal/config`, and the `startNudge`/`drainNudge` helpers that run the passive update-check goroutine in parallel with the resolved verb. The flow is resolve-then-execute: `cli.Resolve` walks argv against the root, parses every flag against the merged FlagSet (mutating `&global` via the closure binding), then `Resolved.Execute` runs the leaf with the populated `Global` on ctx.
- `version.go` — the `version` leaf command plus the `version`/`commit`/`date` package vars that goreleaser stamps via `-ldflags "-X main.version=..."`. `--version` / `-V` is rewritten to the `version` verb up front so flag and subcommand share one rendering path.
- `update.go` — the `update` leaf command (`ana update`) that delegates to `internal/update.SelfUpdate` to download, verify, and replace the running binary.
- `main_test.go` — exercises `run` end-to-end with fakes (no live server) and asserts the verb-map shape, version banner, adapter closures, `startNudge` skip predicates, `drainNudge` branches, and the `update` help short-circuit.
