# ana-cli

`ana` is a Go CLI for [TextQL](https://app.textql.com) that speaks the public Connect-RPC endpoints. Module `github.com/highperformance-tech/ana-cli`, Go 1.25, binary at `cmd/ana`. Verbs are implemented as pure dispatch packages under `internal/` that inject a transport/config boundary, so every package stays unit-testable without a live server.

## Directories

- `cmd/ana/` — main package: wires global flags + config into a transport client, builds the verb map, and hands off to `cli.Dispatch`. `--version` short-circuits to the `version` verb so the banner goes through the same code path regardless of entry shape.
- `internal/` — verb packages (one per top-level noun) plus the shared `cli`, `config`, and `transport` primitives. Each verb package owns its Connect-RPC service prefix and its narrow `Deps` struct; nothing here imports `internal/transport` or `internal/config` except `cli` and `profile`.
- `e2e/` — live smoke tests that run real RPCs against `app.textql.com` via the harness in `e2e/harness/`. Opt-in; require `ANA_E2E_*` env vars.
- `docs/` — human-readable planning docs. `features.md` catalogs TextQL surfaces; `cli-readiness.md` grades CLI coverage per surface.
- `api-catalog/` — JSON entries (~90) capturing every observed Connect-RPC request/response. Source of truth for endpoint shapes and known quirks.
- `.claude/` — Claude Code config + the `textql-webapp-probe` skill that captures new endpoints from the browser.
- `.playwright-mcp/` — scratchpad for Playwright capture artifacts. Gitignored, prune freely.
- `.github/workflows/` — CI (`ci.yml` + release pipeline). Docs-only PRs skip the heavy jobs; see README § "CI scope".
- `install.sh` — curl-friendly installer that fetches the matching `ana_<version>_<os>_<arch>` archive, verifies the sha256, and drops `ana` on `PATH`.
- `.goreleaser.yml`, `release-please-config.json`, `.release-please-manifest.json` — release pipeline config. Conventional commits drive release-please → goreleaser.
- `CONTRIBUTING.md`, `SECURITY.md` — external-contributor onboarding + security disclosure policy. Wired into `.github/pull_request_template.md`.

## Workflow at a glance

- Build / test / cover: `make build`, `make test`, `make cover` (100% coverage gate on `./internal/...`).
- Lint: `make lint` (gofmt, go vet, staticcheck).
- Local release smoke: `make release-local` (`goreleaser check` + `--snapshot`).
- Live smoke tests: `make e2e` (requires `ANA_E2E_ENDPOINT`, `ANA_E2E_TOKEN`, `ANA_E2E_EXPECT_ORG`; see `e2e/README.md`).
- `make help` — self-documenting list of every target.
- Capture a new endpoint: run the `textql-webapp-probe` skill — output lands in `.playwright-mcp/`, then gets emitted into `api-catalog/`.
