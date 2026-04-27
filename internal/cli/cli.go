// Package cli provides argument-dispatch glue for the ana CLI. It defines the
// Command interface, an IO struct carrying stdio/env/clock dependencies, the
// Group helper that stitches together a verb tree, and the resolve-then-parse
// pipeline (Resolve + Dispatch) that walks argv against that tree, parses
// every flag against a single merged FlagSet, and hands off to the resolved
// leaf. The package is pure dispatch logic — it has no dependency on
// transport or config.
//
// The flag pipeline mirrors mainstream "scoped flag set" CLIs (Cobra, Click,
// urfave-cli, clap): each *Group may declare persistent flags via its Flags
// closure that descendant leaves inherit, and each leaf may declare local
// flags by implementing Flagger. Names a leaf re-declares automatically
// SHADOW the ancestor declaration of the same name — see internal/cli/resolve.go
// for the mechanism.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// IO carries the ambient dependencies a Command needs: standard streams, an
// environment accessor, and a clock. Pass this through to subcommands rather
// than reaching for package globals so tests can inject fakes.
type IO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    func(string) string
	Now    func() time.Time
}

// DefaultIO returns an IO backed by os.Stdin/Stdout/Stderr, os.Getenv, and
// time.Now.
func DefaultIO() IO {
	return IO{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Env:    os.Getenv,
		Now:    time.Now,
	}
}

// Command is the consumer interface implemented by every verb or subcommand.
// Run receives the args remaining after the verb path has been consumed —
// pure positionals (the resolver has already routed every flag token to its
// rightful FlagSet target).
type Command interface {
	Run(ctx context.Context, args []string, io IO) error
	Help() string
}

// Flagger is the opt-in declaration interface for leaves with flags. The
// resolver calls Flags(fs) on the resolved leaf BEFORE walking the ancestor
// path, so a name a leaf declares automatically shadows the same name in any
// ancestor Group's Flags closure (the resolver merges ancestor declarations
// onto fs only if the name isn't already present).
//
// A leaf without flags simply omits this method.
type Flagger interface {
	Flags(fs *flag.FlagSet)
}

// Group is a verb-tree node. Children dispatches its leading positional
// argument to a named child Command (which may itself be a *Group, enabling
// nested verbs like `ana chat send …`).
//
// Flags, if set, declares persistent flags that every descendant leaf
// inherits. The closure runs against the resolver's merged FlagSet via a
// shadow set so a leaf that re-declares the same name wins automatically —
// callers can use raw fs.StringVar / fs.BoolVar / fs.IntVar without
// worrying about the stdlib redeclaration panic.
type Group struct {
	Summary  string
	Flags    func(*flag.FlagSet)
	Children map[string]Command
}

// Run dispatches as a Command — the path used when a Group is registered as
// a child of another Group. Resolve handles the descent walk; Run is a thin
// wrapper that re-enters the resolver rooted at this Group so callers (and
// tests) can treat any Group as a self-contained dispatcher.
//
// Empty args or an explicit help token prints the group's help and returns
// ErrHelp. An unknown child name returns ErrUsage.
func (g *Group) Run(ctx context.Context, args []string, stdio IO) error {
	if len(args) == 0 || IsHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, g.Help())
		return ErrHelp
	}
	res, err := Resolve(g, args)
	if err != nil {
		if errors.Is(err, ErrHelp) {
			renderResolvedHelp(res, g, stdio)
			return ErrHelp
		}
		ReportUsageError(res, g, err, stdio.Stderr)
		return errors.Join(err, ErrReported)
	}
	// Mirror Dispatch: stash Global from the merged FlagSet so a leaf calling
	// GlobalFrom(ctx) sees the parsed root persistent flags. Only install if
	// the caller hasn't already supplied one — Group.Run is reachable from
	// tests that pre-seed Global on ctx, and overwriting their value would
	// change behavior the existing FlagSet-preservation contract codifies.
	if GlobalFrom(ctx) == (Global{}) {
		ctx = WithGlobal(ctx, globalFromFlagSet(res.MergedFS))
	}
	// Resolved.Execute owns group-prefix help, leaf invocation, and
	// leaf-internal-usage-error annotation in one place — Group.Run delegates
	// to it so the convention can't drift between dispatch entry points.
	return res.Execute(ctx, stdio)
}

// renderFlagsAsText enumerates fs's flags sorted by name and renders one
// `  --name <type>   usage (default: X)` row per flag. Returns "" if fs has
// no flags. The trailing newline is included so callers can Fprint without
// worrying about terminator placement.
func renderFlagsAsText(fs *flag.FlagSet) string {
	type row struct {
		name, typ, usage, def string
	}
	var rows []row
	fs.VisitAll(func(f *flag.Flag) {
		typ, usage := flag.UnquoteUsage(f)
		rows = append(rows, row{name: f.Name, typ: typ, usage: usage, def: f.DefValue})
	})
	if len(rows) == 0 {
		return ""
	}
	slices.SortFunc(rows, func(a, b row) int { return strings.Compare(a.name, b.name) })
	nameWidth := 0
	for _, r := range rows {
		w := len(r.name)
		if r.typ != "" {
			w += 1 + len(r.typ)
		}
		if w > nameWidth {
			nameWidth = w
		}
	}
	var b strings.Builder
	for _, r := range rows {
		head := "--" + r.name
		if r.typ != "" {
			head += " " + r.typ
		}
		fmt.Fprintf(&b, "  %-*s   %s", nameWidth+2, head, r.usage)
		if r.def != "" && r.def != "false" && r.def != "0" {
			fmt.Fprintf(&b, " (default: %s)", r.def)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Help renders the group's summary (if set) followed by a sorted, two-column
// listing of child commands and the first line of each child's own Help().
// When Flags is set, a trailing "Flags:" block enumerates the group-level
// flags so `ana <group> --help` surfaces inheritable flags even when the
// user hasn't drilled into a leaf.
func (g *Group) Help() string {
	var b strings.Builder
	if g.Summary != "" {
		b.WriteString(g.Summary)
		b.WriteString("\n\n")
	}
	b.WriteString("Commands:\n")
	names := make([]string, 0, len(g.Children))
	for name := range g.Children {
		names = append(names, name)
	}
	slices.Sort(names)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		first := FirstLine(g.Children[n].Help())
		fmt.Fprintf(&b, "  %-*s   %s\n", width, n, first)
	}
	if g.Flags != nil {
		fs := flag.NewFlagSet("help", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		g.Flags(fs)
		if block := renderFlagsAsText(fs); block != "" {
			b.WriteString("\nFlags:\n")
			b.WriteString(block)
		}
	}
	// Trim trailing newline so callers can Fprintln without doubling blanks.
	return strings.TrimRight(b.String(), "\n")
}

// IsHelpArg reports whether s is one of the recognized help tokens.
func IsHelpArg(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

// FirstLine returns the first line of s (without the newline). Exported so
// verb packages that render streaming/multi-line payloads one-row-per-frame
// can reuse the same definition the help renderer uses.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
