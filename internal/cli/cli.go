// Package cli provides argument-dispatch glue for the ana CLI. It defines the
// Command interface, an IO struct carrying stdio/env/clock dependencies, and a
// Group helper that dispatches to named child Commands. The package is pure
// dispatch logic — it has no dependency on transport or config.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
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
// Run receives the args remaining after its own name has been consumed.
type Command interface {
	Run(ctx context.Context, args []string, io IO) error
	Help() string
}

// Group is a Command that dispatches its first argument to a named child
// Command. A Group can itself be registered as a child, enabling nested verbs
// (e.g. `ana chat send ...`).
type Group struct {
	Summary  string
	Children map[string]Command
}

// Run dispatches to a child command. With no args or a help flag it prints
// Help() to stdout and returns ErrUsage. An unknown child name writes to
// stderr and also returns ErrUsage.
func (g *Group) Run(ctx context.Context, args []string, stdio IO) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Fprintln(stdio.Stdout, g.Help())
		return ErrUsage
	}
	name := args[0]
	child, ok := g.Children[name]
	if !ok {
		fmt.Fprintf(stdio.Stderr, "unknown subcommand: %s\n", name)
		fmt.Fprintln(stdio.Stderr, g.Help())
		return fmt.Errorf("unknown subcommand %q: %w", name, ErrUsage)
	}
	return child.Run(ctx, args[1:], stdio)
}

// Help renders the group's summary (if set) followed by a sorted, two-column
// listing of child commands and the first line of each child's own Help().
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
	sort.Strings(names)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		first := firstLine(g.Children[n].Help())
		fmt.Fprintf(&b, "  %-*s   %s\n", width, n, first)
	}
	// Trim trailing newline so callers can Fprintln without doubling blanks.
	return strings.TrimRight(b.String(), "\n")
}

// isHelpArg reports whether s is one of the recognized help tokens.
func isHelpArg(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

// firstLine returns the first line of s (without the newline).
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
