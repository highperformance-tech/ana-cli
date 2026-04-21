# Contributing

Thanks for your interest in `ana-cli`. External contributions are
welcome — this document covers the basics.

## Prerequisites

- Go 1.25 or newer.
- `make`.
- `goreleaser` only if you want to run `make release-local`.

## Building and testing

```bash
make build    # -> ./bin/ana
make test     # go test -race ./...
make cover    # enforces 100% coverage on ./internal/...
make lint     # gofmt, go vet, staticcheck
```

`make cover` runs staticcheck-pinned dev tools. Run `make deps` once to
install them.

End-to-end tests live in `e2e/` and require a live TextQL endpoint
plus a token. They are opt-in; see [`e2e/README.md`](e2e/README.md).
You do not need them to land most PRs.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
Commit subjects drive the release pipeline (release-please →
GoReleaser), so get the prefix right:

- `feat:` — new verb, flag, or user-visible behavior.
- `fix:` — bug fix.
- `refactor:` — internal change, no behavior change.
- `test:` — test-only change.
- `docs:` — docs, README, CLAUDE.md.
- `chore:` / `ci:` — tooling, pipeline, dependencies.

Scope is optional but helpful (e.g. `fix(auth): ...`).

## Pull requests

- Branch off `main`, open the PR against `main`.
- One logical change per PR. Split refactors from features.
- CI runs lint, test on linux/macos/windows, a 100% coverage gate on
  `./internal/...`, a build check, and `goreleaser check`. Docs-only
  PRs skip the heavy jobs automatically.
- Squash merge is the house style — the PR title becomes the commit
  subject, so make it conform to Conventional Commits.

## Adding a new verb

Each top-level verb lives in its own package under `internal/`. The
pattern:

1. Declare a narrow `Deps` struct; do not import `internal/transport`
   or `internal/config` — `cmd/ana/main.go` wires those in.
2. Build a `cli.Group` from `New(deps)` and register subcommands.
3. Write tests using the fake `Deps` pattern — see
   `internal/feed/feed_test.go` for the convention.

Coverage is gated at 100% on `./internal/...`, so every branch needs
a test.

## Questions

Open a [discussion](https://github.com/highperformance-tech/ana-cli/discussions)
or a draft PR. For security issues, see [SECURITY.md](SECURITY.md).
