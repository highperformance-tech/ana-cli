package cli

import (
	"flag"
	"fmt"
)

// ParseFlags parses args into fs, tolerating positional arguments interleaved
// with flags. Go's stdlib FlagSet.Parse stops at the first non-flag token,
// which silently drops any flags that follow — so `cmd <id> --flag v` would
// parse the positional but ignore --flag. This helper iterates: parse,
// collect a non-flag token, parse the remainder, repeat. A final Parse with
// a "--" separator then re-seeds fs.Args() with the collected positionals so
// callers can read them through the normal flag API.
//
// On any underlying Parse failure the error is wrapped with ErrUsage so the
// root dispatcher maps it to exit code 1.
func ParseFlags(fs *flag.FlagSet, args []string) error {
	var positional []string
	remaining := args
	for {
		if err := fs.Parse(remaining); err != nil {
			return fmt.Errorf("%s: %w: %w", fs.Name(), err, ErrUsage)
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		remaining = fs.Args()[1:]
	}
	if len(positional) > 0 {
		// Re-seed fs.Args() via a "--" terminator. The stdlib treats
		// everything after "--" as positional, so this second Parse cannot
		// fail on flag validation.
		_ = fs.Parse(append([]string{"--"}, positional...))
	}
	return nil
}

// FlagWasSet reports whether fs saw name as an explicit argument. Partial-
// update verbs use it to distinguish "user left this alone" (keep server's
// current value) from "user passed the zero value on purpose". Uses the
// stdlib-documented `fs.Visit` idiom, which only traverses flags that were
// actually set; an unknown name is reported as not-set rather than
// panicking.
func FlagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
