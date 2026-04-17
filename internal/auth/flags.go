package auth

import (
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newFlagSet builds a FlagSet configured the way every leaf command wants:
// continue-on-error parsing (no os.Exit), all output suppressed (each command
// prints its own Help() via the Command interface), and the supplied name so
// error messages stay attributable.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// usageErrf wraps cli.ErrUsage with a formatted message so Dispatch/ExitCode
// can detect it via errors.Is and return exit code 1. Callers pass the verb
// name + the specific complaint (e.g. "--name is required").
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// parseFlags invokes fs.Parse and wraps any error with cli.ErrUsage. Returning
// the usage sentinel here means every leaf command doesn't have to replicate
// the boilerplate — they just `return parseFlags(...)` on failure.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
	}
	return nil
}
