# e2e/testdata

Static fixtures used by the live-smoke harness and its templated reports.

## Files

- `manual-revert.template.md` — template the harness's `ManualRevertLog` renders when a mutation could not be auto-reverted. Placeholders (`<ts>`, `<name>`, `<id>`, `{what}`, `{reason}`, `{ts}`) are filled in by `e2e/harness/ledger.go`.
