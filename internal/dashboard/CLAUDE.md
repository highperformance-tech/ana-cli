# internal/dashboard

The `ana dashboard` verb tree: `list`, `folders`, `get`, `spawn`, `health`. Dispatch-only around `Deps.Unary`. No create/update/delete captured yet — see `docs/cli-readiness.md`.

## Files

- `dashboard.go` — `New`, `Deps`, service path prefix.
- `list.go` — `ListDashboards`.
- `folders.go` — `ListDashboardFolders`.
- `get.go` — `GetDashboard`.
- `spawn.go` — `SpawnDashboard` (produces a new dashboard from a template).
- `health.go` — `CheckDashboardHealth`.
- `dashboard_test.go` — shared `fakeDeps` + `TestNew*`/`TestHelp*`.
- `list_test.go` / `folders_test.go` / `get_test.go` / `spawn_test.go` / `health_test.go` — one per source file; `health_test.go` also covers the `healthLabel` helper defined in `health.go`.
