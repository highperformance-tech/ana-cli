# internal/api

The `ana api <path>` verb — authenticated raw-JSON passthrough over the shared transport client. Single leaf, no subcommands. Covers both Connect-RPC (`textql.rpc.public.<Service>/<Method>` or `/rpc/public/...`) and the documented REST API (`/v1/...`) — one verb, two surfaces, distinguished by leading slash.

## Files

- `api.go` — `Deps` (just `DoRaw`), `New` (returns a leaf `cli.Command`, no subcommands), and the `/rpc/public/` prefix.
- `call.go` — the leaf: path dispatch (leading slash verbatim, else prefix-prepend), body resolution (`--data` / `--data-stdin` / default `{}` for POST, nil for GET/HEAD), and the `emitError`/`emitSuccess` split. Non-2xx echoes the server body to stderr; 2xx pretty-prints JSON unless `--raw`.
- `api_test.go`, `call_test.go` — shared fakes + per-source coverage.
