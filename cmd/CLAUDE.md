# cmd

Module entry points for every binary this repo produces. Today there is exactly one.

## Children

| Path | Contents |
|------|----------|
| `ana/` | `ana` CLI main package — declares the root verb tree with persistent flags, wires lazy transport+config closures, runs `cli.Resolve` + `Resolved.Execute`. |
