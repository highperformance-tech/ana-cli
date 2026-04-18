# internal/playbook

The `ana playbook` verb tree: `list`, `get`, `reports`, `lineage`. Readonly surface — no create/update/run captured yet. Dispatch-only around `Deps.Unary`.

## Files

- `playbook.go` — `New`, `Deps`, service path prefix.
- `list.go` — `GetPlaybooks`.
- `get.go` — `GetPlaybook`.
- `reports.go` — `GetPlaybookReports` + `GetChatReportsSummary`.
- `lineage.go` — `GetPlaybookLineage`.
- `playbook_test.go` — fake `Unary` covers each subcommand.
