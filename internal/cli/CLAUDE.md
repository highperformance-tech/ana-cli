# internal/cli

Argument-dispatch core shared by every verb. Defines the `Command` interface, the `Group` verb-tree node, the `IO` struct, the `Flagger` declaration interface, and a Cobra-style resolve-then-parse pipeline (`Resolve` + `Dispatch`) that walks argv against the verb tree, parses every flag against a single merged `*flag.FlagSet`, and hands off to the resolved leaf. Pure dispatch logic — no dependency on `internal/transport` or `internal/config`.

The flag pipeline mirrors mainstream "scoped flag set" CLIs (Cobra, Click, urfave-cli, clap): each `*Group` may declare persistent flags via its `Flags` closure that descendant leaves inherit; each leaf may declare local flags by implementing `Flagger`. Names a leaf re-declares automatically SHADOW the same name in any ancestor `Group.Flags` closure — the resolver registers the leaf's flags first on the merged FlagSet, then runs each ancestor closure on a private shadow set and copies only the non-clashing entries onto the merged set. That structurally prevents the "global flag stomps leaf flag" class of bug (e.g. `ana profile add --endpoint X` saving the default endpoint).

## Files

- `cli.go` — public surface: `Command`, `IO`, `Flagger`, `Group` (verb-tree node with persistent `Flags`), and the small text helpers (`renderFlagsAsText`, `IsHelpArg`, `FirstLine`). `Group.Run` re-enters `Resolve` + `Resolved.Execute` so any Group can self-dispatch.
- `resolve.go` — the resolver (`Resolve` + `Resolved` + `Resolved.Execute`) and the ctx helpers (`WithFlagSet`, `FlagSetFrom`, `globalFromFlagSet`). `Execute` is the single chokepoint that runs the leaf and auto-annotates leaf-returned `ErrUsage` with the leaf's help, so every dispatch entry point inherits the modern-CLI error layout.
- `dispatch.go` — root entry (`Dispatch`) plus help/usage rendering: `RootHelp`, `RenderResolvedHelp`, and `ReportUsageError` (writes "<error>\\n\\n<help>" and strips the trailing `: usage` sentinel — repeated, to cover stdlib-flag double-wrap from custom `flag.Value.Set` errors).
- `root.go` — `Global` (the four root persistent flags) + `WithGlobal` / `GlobalFrom` ctx helpers + the `parseFlagToken` token classifier.
- `flags.go` — `ParseFlags` (interleaved-positional-tolerant wrapper), `RequireFlags`, `RequireNoPositionals`, `RequireMaxPositionals`, `FlagWasSet`, and three typed `flag.Value` constructors (`EnumFlag`, `IntListFlag`, `SinceFlag`).
- `token.go` — the `Token` named string whose `String`/`Format` always render via `RedactToken`.
- `helpers.go` — verb-package helpers: `NewFlagSet`, `UsageErrf`, `WriteJSON`, `Remarshal`, `RenderOutput`, `RequireStringID`, `RequireIntID`, `RenderTwoCol`, `ReadToken`, `ReadPassword`, `NewTableWriter`, `FirstLine`, `DashIfEmpty`, `RedactToken`.
- `errors.go` — `ErrUsage`, `ErrHelp`, `ErrReported`, and `ExitCode` (sentinel chain + `auth.ErrNotLoggedIn` → process exit code).
- `cli_test.go`, `flags_test.go`, `helpers_test.go`, `resolve_test.go` — package tests; 100% coverage gate.
