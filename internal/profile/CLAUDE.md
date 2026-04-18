# internal/profile

The `ana profile` verb tree: `list`, `add`, `use`, `remove`, `show`. Profiles let a user flip between TextQL orgs (API keys are org-scoped) with `ana profile use`. Unlike `internal/auth`, this package intentionally imports `internal/config` — managing profiles IS the whole config surface, so `Deps` speaks `config.Config` directly.

## Files

- `profile.go` — `New`, `Deps` (LoadCfg/SaveCfg/ConfigPath closures).
- `add.go` — create a new profile from `--endpoint`/`--token` (or prompted values) and optionally switch to it.
- `use.go` — switch the `active` pointer to a named profile.
- `remove.go` — delete a profile; clears `active` if the current target is removed.
- `list.go` / `show.go` — inspect configured profiles.
- `profile_test.go` — end-to-end round-trip against a `t.TempDir` config for every subcommand, including the remove → clear-active edge case.
