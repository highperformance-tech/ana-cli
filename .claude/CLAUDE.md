# .claude

Claude Code config + skills for this repo.

## Files

- `settings.json` — team-shared Claude Code settings (permissions, plugin config).
- `settings.local.json` — personal overrides (gitignored).
- `skills/` — project-local skills:
  - `textql-webapp-probe` — drives Playwright to capture new TextQL API endpoints.
  - `claude-md-maintenance` — audits CLAUDE.md files for staleness; wired into `.git/hooks/pre-commit` so every commit re-runs the audit.
