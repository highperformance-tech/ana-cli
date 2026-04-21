# CI scope

PRs are gated by a single required check, `CI Complete`. To keep runner time
proportional to impact, docs-only PRs skip the Go lint / test / build /
goreleaser jobs and `CI Complete` reports green immediately. A PR counts as
"code" when it touches any of:

- `**/*.go`, `go.mod`, `go.sum`
- `Makefile`, `.goreleaser.yml`, `install.sh`
- `.github/workflows/**`

Everything else — `README.md`, `LICENSE`, `docs/**`, `api-catalog/**`,
`.claude/**`, `.gitignore` — skips the heavy jobs. Release-please likewise
ignores doc-only merges on `main`.
