# internal/api

The `ana api <path>` verb — authenticated raw-JSON passthrough over the shared transport client. Single leaf, no subcommands. Covers both Connect-RPC (`textql.rpc.public.<Service>/<Method>` or `/rpc/public/...`) and the documented REST API (`/v1/...`) — one verb, two surfaces, distinguished by leading slash.

## Files

- `api.go` — `Deps` (single `DoRaw` function field), `New` (returns a leaf `cli.Command`, not a `*cli.Group` — no subcommands), and the `/rpc/public/` prefix constant.
- `call.go` — the leaf: flag parsing, path dispatch (leading slash → verbatim; else prefix-prepend), body resolution (`--data` / `--data-stdin` / default `{}` for POST, `nil` for GET/HEAD), and the `emitError`/`emitSuccess` split. Non-2xx writes the server body to stderr and returns an `api: HTTP <status>` summary error; 2xx writes pretty JSON to stdout (fallthrough to raw if the body isn't valid JSON; `--raw` skips pretty-print entirely).
- `api_test.go` — shared `fakeDeps` + `TestNew*`/`TestHelp*`.
- `call_test.go` — per-source test file covering both path forms, every body-resolution branch, mutual-exclusion, non-2xx stderr + trailing-newline branches, and the raw/pretty/non-JSON 2xx paths.
