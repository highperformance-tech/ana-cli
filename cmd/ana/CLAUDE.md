# cmd/ana

The `ana` binary's main package. Pure wiring: reads global flags + config, constructs a `transport.Client`, assembles the verb map by injecting adapted `Deps` into each `internal/<verb>` package, and hands off to `cli.Dispatch`. All domain logic lives under `internal/`.

## Files

- `main.go` — `main` + `run` (the testable entrypoint with injectable args/stdio/env) and the `buildVerbs`/`authDeps`/`profileDeps`/`chatDeps` adapters. Also owns `newUUID` (used for chat `cellId`s) and the projection `profileToAuthConfig` that keeps `internal/auth` from importing `internal/config`.
- `version.go` — the `version` leaf command plus the `version`/`commit`/`date` package vars that goreleaser stamps via `-ldflags "-X main.version=..."`. `--version` / `-V` is rewritten to the `version` verb up front so flag and subcommand share one rendering path.
- `main_test.go` — exercises `run` end-to-end with fakes (no live server) and asserts the verb-map shape, version banner, and all adapter closures.
