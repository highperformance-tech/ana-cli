package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
)

// Dispatch is the root entry point. It parses global flags, stashes them in
// ctx, then routes to the matching verb. An explicit help token returns
// ErrHelp (exit 0); an empty verb or parse error returns ErrUsage (exit 1).
func Dispatch(ctx context.Context, verbs map[string]Command, args []string, stdio IO) error {
	// A bare help token anywhere up front short-circuits flag parsing so users
	// can discover commands without first fixing any flag validation errors.
	if len(args) > 0 && isHelpArg(args[0]) {
		RootHelp(stdio.Stdout, verbs)
		return ErrHelp
	}
	global, rest, err := ParseGlobal(args)
	if err != nil {
		fmt.Fprintln(stdio.Stderr, err)
		RootHelp(stdio.Stderr, verbs)
		return fmt.Errorf("%w: %s", ErrUsage, err.Error())
	}
	ctx = WithGlobal(ctx, global)

	if len(rest) == 0 {
		RootHelp(stdio.Stdout, verbs)
		return ErrHelp
	}
	if isHelpArg(rest[0]) {
		RootHelp(stdio.Stdout, verbs)
		return ErrHelp
	}

	name := rest[0]
	verb, ok := verbs[name]
	if !ok {
		fmt.Fprintf(stdio.Stderr, "unknown command: %s\n", name)
		RootHelp(stdio.Stderr, verbs)
		return fmt.Errorf("unknown command %q: %w", name, ErrUsage)
	}
	return verb.Run(ctx, rest[1:], stdio)
}

// RootHelp writes a sorted listing of the top-level verbs to w, each followed
// by the first line of its own Help().
func RootHelp(w io.Writer, verbs map[string]Command) {
	names := make([]string, 0, len(verbs))
	for name := range verbs {
		names = append(names, name)
	}
	slices.Sort(names)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	fmt.Fprintln(w, "Usage: ana [global flags] <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, n := range names {
		fmt.Fprintf(w, "  %-*s   %s\n", width, n, FirstLine(verbs[n].Help()))
	}
}
