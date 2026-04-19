# internal/cli

Argument-dispatch core shared by every verb. Defines the `Command` interface, the `Group` dispatcher, the `IO` struct, and the root-level `Global` flags; parses argv (including positional-interleaved flags) and maps sentinel errors to exit codes. Pure dispatch logic — no dependency on `internal/transport` or `internal/config`.

## Files

- `cli.go` — `Command`, `IO`, `DefaultIO`, and `Group` (nested-verb dispatcher with auto-generated help listing).
- `dispatch.go` — `Dispatch` (root entry: short-circuits help, parses globals, routes to the matching verb) and `RootHelp`.
- `root.go` — `Global` shape, `WithGlobal`/`GlobalFrom` context helpers, and `ParseGlobal` (strips known root flags from argv before verb dispatch).
- `flags.go` — `ParseFlags`, which tolerates positional args interleaved with flags (stdlib `FlagSet.Parse` stops at the first non-flag, silently dropping later flags).
- `helpers.go` — shared verb-package helpers extracted from per-verb duplicates: `NewFlagSet`, `UsageErrf`, `WriteJSON`, `Remarshal`, `RequireStringID`, `RequireIntID`, `RenderTwoCol`, `ReadToken` (shared login/profile-add stdin reader with JWT-sized buffer boost), `NewTableWriter` (canonical tabwriter config). Phase 0 of the shared-cli-kit refactor; Phases 1–10 migrate each verb package over and delete its local copies.
- `errors.go` — `ErrUsage`, `ErrHelp`, and `ExitCode` (maps these plus `auth.ErrNotLoggedIn` to the process exit code).
- `cli_test.go`, `flags_test.go`, `helpers_test.go` — cover dispatch, help rendering, flag parsing, exit-code mapping, and every branch of the shared verb helpers. `//lint:ignore SA1012` directives mark the intentional nil-context coverage cases.
