# docs

Human-readable planning docs. Read these before touching `api-catalog/`.

## Files

- `features.md` — inventory of TextQL surfaces (auth, chat, connectors, dashboards, playbooks, etc.), service/endpoint counts, verified enums, and per-surface quirks. "Last verified" dated.
- `cli-readiness.md` — CLI-implementer's view of `features.md`. TL;DR, per-surface confidence table (✅/🟡/❗), enum catalog, known quirks, first-cut command shape, and prioritized follow-up probes.
- `ci-scope.md` — which file globs count as "code" and trigger the full CI matrix vs docs-only skip.
- `windows-smartscreen.md` — why signed release binaries run clean but `go install` / `make build` builds get SmartScreen-blocked, and how to unblock.

## Subdirectories

- `prompts/` — self-contained prompt briefs for fresh Claude Code sessions driving larger multi-commit workstreams.
