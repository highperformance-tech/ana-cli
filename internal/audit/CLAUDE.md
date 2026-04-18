# internal/audit

The `ana audit` verb tree: `tail`. Wraps `AuditLogService.ListAuditLogs` with a `--since` flag; the proto package uses the underscored name `textql.rpc.public.audit_log`, so the service path does too. `Deps` carries an injectable `Now` so `--since` tests are deterministic.

## Files

- `audit.go` — `New`, `Deps` (Unary + Now), service path constant.
- `tail.go` — `tail` subcommand: parses `--since` (relative durations + absolute RFC3339), emits audit records as JSON lines. Action strings are snake_case (`api_key.created`, etc.).
- `audit_test.go` — fake `Unary` + fixed `Now` exercise the `--since` math plus the JSON rendering.
