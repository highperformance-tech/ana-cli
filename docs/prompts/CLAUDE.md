# docs/prompts

Self-contained prompt briefs used to kick off fresh Claude Code sessions for
larger refactors or multi-commit workstreams. Each file is written so it can be
pasted into a cold session and drive the work end-to-end without prior
conversation context.

## Files

- `e2e-comprehensive-coverage.md` — brief for the "smoke every verb/leaf/flag
  across the CLI" workstream; base branch `feature/e2e-snowflake-coverage`,
  ledger-backed cleanup, `h.Run`/`h.RunStdin` only, one verb per commit.
