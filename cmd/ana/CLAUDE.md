# cmd/ana

The `ana` binary's main package. Pure wiring: declares the root `*cli.Group` with persistent flags (`--json`, `--endpoint`, `--token-file`, `--profile`), assembles the verb tree by injecting lazy `transport.Client` + config closures into each `internal/<verb>`'s `Deps`, then runs `cli.Resolve` + `Resolved.Execute`. All domain logic lives under `internal/`.

## Files

- `main.go` — `main` + the testable `run`, the root `*cli.Group` with the four persistent flags, and `lazyState` + `buildVerbs` that defer config/transport construction until a verb actually needs them. Also hosts `newUUID`, `profileToAuthConfig` (the projection that keeps `internal/auth` away from `internal/config`), and the `startNudge`/`drainNudge` pair that runs the passive update-check in parallel with the resolved verb.
- `version.go` — the `version` leaf and the goreleaser-stamped package vars. `--version` / `-V` rewrites to the `version` verb so flag and subcommand share one render path.
- `update.go` — the `update` leaf; thin wrapper around `internal/update.SelfUpdate`.
- `main_test.go` — end-to-end `run` tests with fakes (no live server).
