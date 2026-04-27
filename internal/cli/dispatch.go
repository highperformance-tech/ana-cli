package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Dispatch is the root entry point. It walks args against the verb tree under
// root, parses every flag token (root persistent + intermediate group
// persistent + leaf local) against a single merged FlagSet, stashes the
// resulting Global in ctx, and hands off to the resolved leaf.
//
// A bare help token (`help` / `-h` / `--help`) at the very front prints the
// root group's help and returns ErrHelp; deeper help tokens are handled by
// Resolve and rendered against the resolved leaf or group.
//
// Errors:
//   - ErrHelp (exit 0): help was requested.
//   - usage errors (exit 1): unknown verb or unknown / malformed flag, also
//     wrapped with ErrReported so main() doesn't double-print.
//   - any error returned by leaf.Run: passed through unchanged.
func Dispatch(ctx context.Context, root *Group, args []string, stdio IO) error {
	if root == nil {
		return fmt.Errorf("dispatch: nil root group")
	}
	// Bare help token at the front is the historical fast path. We could let
	// Resolve handle it, but printing root help directly keeps the empty-args
	// case and the explicit help-token case symmetrical.
	if len(args) == 0 {
		fmt.Fprintln(stdio.Stdout, RootHelp(root))
		return ErrHelp
	}
	if IsHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, RootHelp(root))
		return ErrHelp
	}

	res, err := Resolve(root, args)
	if err != nil {
		if errors.Is(err, ErrHelp) {
			renderResolvedHelp(res, root, stdio)
			return ErrHelp
		}
		// Modern-CLI convention: error message first, then the help text for
		// the deepest scope the resolver reached so the user sees the syntax
		// they were trying to use.
		ReportUsageError(res, root, err, stdio.Stderr)
		return errors.Join(err, ErrReported)
	}

	ctx = WithGlobal(ctx, globalFromFlagSet(res.MergedFS))
	ctx = WithFlagSet(ctx, res.MergedFS)
	// Resolved.Execute is the single chokepoint that handles both group-prefix
	// help rendering and leaf-internal-usage-error annotation; Dispatch never
	// calls Leaf.Run directly so the modern-CLI convention can't drift.
	return res.Execute(ctx, stdio)
}

// shouldAttachUsageHelp reports whether a verb-returned error is a bare
// usage error that the dispatcher should annotate with help. ErrHelp,
// already-reported errors, and non-usage errors are passed through unchanged.
func shouldAttachUsageHelp(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrHelp) || errors.Is(err, ErrReported) {
		return false
	}
	return errors.Is(err, ErrUsage)
}

// ReportUsageError writes a syntax-error report to w in the modern-CLI
// convention: the error on the first line, a blank separator, then the help
// text for the deepest scope the resolver reached. A nil res falls back to
// the root help. Callers wrap the returned-from-Run / Resolve error with
// ErrReported so main()'s fallback printer doesn't double-emit it.
//
// The trailing `: usage` sentinel suffix that errors.Is(err, ErrUsage) leaves
// in err.Error() is stripped from the printed line — it is a routing tag for
// callers, not user-facing diagnostic text.
func ReportUsageError(res *Resolved, root *Group, err error, w io.Writer) {
	fmt.Fprintln(w, trimUsageSuffix(err.Error()))
	fmt.Fprintln(w)
	RenderResolvedHelp(res, root, w)
}

// trimUsageSuffix removes every trailing ": usage" tag that wrapping with
// ErrUsage adds to err.Error(). The double-wrap case happens for custom
// flag.Value.Set methods that return UsageErrf — stdlib flag flattens the
// inner error via %v (preserving its ": usage" text) and then ParseFlags
// wraps the result with ErrUsage again, producing two trailing tags.
func trimUsageSuffix(s string) string {
	const suffix = ": " + "usage" // string literal so refactors of ErrUsage's text are caught at test time
	for strings.HasSuffix(s, suffix) {
		s = s[:len(s)-len(suffix)]
	}
	return s
}

// renderResolvedHelp prints the help text appropriate for whatever the
// resolver landed on when a `--help` / `-h` / `help` token appeared in argv.
// Thin wrapper that targets stdio.Stdout; the exported RenderResolvedHelp
// has the writer-injection signature for cmd/ana.
func renderResolvedHelp(res *Resolved, root *Group, stdio IO) {
	RenderResolvedHelp(res, root, stdio.Stdout)
}

// RenderResolvedHelp is the exported variant of renderResolvedHelp so
// cmd/ana can render leaf+ancestor help without duplicating the merged-FS
// formatting logic. For a Group leaf the group's Help is printed (which
// already contains the Commands listing + persistent Flags block); for a
// non-Group leaf the leaf's Help is printed followed by the merged-FS Flags
// block (every flag in scope — leaf + ancestor persistent). A nil res falls
// back to RootHelp(root).
func RenderResolvedHelp(res *Resolved, root *Group, w io.Writer) {
	if res == nil {
		fmt.Fprintln(w, RootHelp(root))
		return
	}
	if g, ok := res.Leaf.(*Group); ok {
		fmt.Fprintln(w, g.Help())
		return
	}
	fmt.Fprintln(w, res.Leaf.Help())
	if res.MergedFS != nil {
		if block := renderFlagsAsText(res.MergedFS); block != "" {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Flags:")
			fmt.Fprint(w, block)
		}
	}
}

// RootHelp renders the root group's help — the sorted listing of top-level
// verbs each followed by its summary line, plus the trailing Flags block
// describing the four root persistent flags.
func RootHelp(root *Group) string {
	if root == nil {
		return ""
	}
	return root.Help()
}
