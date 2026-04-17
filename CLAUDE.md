# ana-cli

Reverse-engineered API catalog + planning docs for a CLI targeting `app.textql.com`. No CLI source yet — this repo is the specification the CLI will be built against.

## Directories

- `api-catalog/` — one JSON file per captured Connect-RPC endpoint (~85 entries). Request/response samples, inferred schemas, quirks. Source of truth for endpoint shapes.
- `docs/` — human-readable guides: feature inventory (`features.md`) and CLI-readiness review (`cli-readiness.md`). Start here before coding.
- `.claude/` — Claude Code configuration: permissions (`settings.json`, `settings.local.json`) and the `textql-webapp-probe` skill that drives the browser to capture new endpoints.
- `.playwright-mcp/` — raw Playwright capture artifacts (page snapshots, network dumps, emit scripts). Scratchpad; safe to prune between probe sessions. Gitignored.

## Workflow at a glance

1. Read `docs/features.md` for the surface you care about.
2. Look up the endpoint in `api-catalog/` (grep by `<Service>__<Method>`).
3. If missing, run the `textql-webapp-probe` skill to capture it — outputs land in `.playwright-mcp/`, then get emitted into `api-catalog/`.
