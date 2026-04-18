# internal/dashboard

The `ana dashboard` verb tree: `list`, `folders`, `get`, `spawn`, `health`. Dispatch-only around `Deps.Unary`. No create/update/delete captured yet — see `docs/cli-readiness.md`.

## Files

- `dashboard.go` — `New`, `Deps`, service path prefix.
- `list.go` — `ListDashboards`.
- `folders.go` — `ListDashboardFolders`.
- `get.go` — `GetDashboard`.
- `spawn.go` — `SpawnDashboard` (produces a new dashboard from a template).
- `health.go` — `CheckDashboardHealth`.
- `dashboard_test.go` — fake `Unary` covers each subcommand.
