# textql-webapp-probe

Skill that drives Playwright MCP against `app.textql.com` to capture Connect-RPC endpoints and emit catalog entries.

## Files

- `SKILL.md` — skill frontmatter + workflow entry point. Points at `references/` for details.
- `references/` — longer-form docs: workflow, network-capture conventions, catalog schema, known-surfaces log.
- `scripts/` — bash helpers: `normalize_request.sh` (redact + shape for catalog), `diff_catalog.sh` (compare captures against existing entries).
