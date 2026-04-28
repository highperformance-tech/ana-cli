# internal/audit

The `ana audit` verb tree: `tail`. Wraps `AuditLogService.ListAuditLogs` with a `--since` flag; the proto package uses the underscored name `textql.rpc.public.audit_log`, so the service path does too. `Deps` carries an injectable `Now` so `--since` tests are deterministic.

## Files

- `audit.go` — `New`, `Deps` (Unary + Now), service path constant.
- `tail.go` — the `tail` subcommand. `--since` is a `cli.SinceFlag` bound to `Deps.Now`; records emit as JSON lines. Action strings are snake_case (`api_key.created`).
- `audit_test.go`, `tail_test.go` — shared fakes + per-source coverage.
