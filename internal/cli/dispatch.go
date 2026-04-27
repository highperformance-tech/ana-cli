package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
		// Print to stderr and append root help so the user can recover.
		fmt.Fprintln(stdio.Stderr, err)
		fmt.Fprintln(stdio.Stderr, RootHelp(root))
		return errors.Join(err, ErrReported)
	}

	// User typed only a group prefix (e.g. `ana profile`) — render that
	// group's help instead of trying to Run a *Group.
	if g, ok := res.Leaf.(*Group); ok {
		fmt.Fprintln(stdio.Stdout, g.Help())
		return ErrHelp
	}

	ctx = WithGlobal(ctx, globalFromFlagSet(res.MergedFS))
	ctx = WithFlagSet(ctx, res.MergedFS)
	return res.Leaf.Run(ctx, res.Args, stdio)
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
