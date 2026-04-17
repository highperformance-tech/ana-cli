package connector

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newFlagSet returns a FlagSet configured for leaf commands: continue-on-error
// so we can wrap parse failures as usage errors, and silenced output so each
// command's own Help() is the sole source of usage text.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// usageErrf wraps cli.ErrUsage with a formatted message so Dispatch/ExitCode
// map it to exit code 1 via errors.Is.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// parseFlags invokes fs.Parse and wraps any error with cli.ErrUsage. Accepts
// positional args interleaved with flags — Go's flag package stops at the
// first non-flag arg, so without this loop `update <id> --name X` would
// leave --name unparsed. After collecting all positionals, a final Parse
// with a `--` terminator restores them to fs.Args()/fs.NArg() so callers
// can read them through the normal API.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var positional []string
	remaining := args
	for {
		if err := fs.Parse(remaining); err != nil {
			return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		remaining = fs.Args()[1:]
	}
	if len(positional) > 0 {
		trailing := append([]string{"--"}, positional...)
		if err := fs.Parse(trailing); err != nil {
			return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
		}
	}
	return nil
}

// atoiID parses a positional <id> arg into an int, returning a usage error if
// the input is empty or non-numeric. connector IDs are integers in the API.
func atoiID(verb, s string) (int, error) {
	if s == "" {
		return 0, usageErrf("%s: <id> positional argument required", verb)
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0, usageErrf("%s: <id> must be an integer: %v", verb, err)
	}
	return id, nil
}

// writeJSON pretty-prints v to w with the indent convention used across the
// CLI (2 spaces, trailing newline via Encoder).
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal round-trips src through JSON into dst, letting commands have both
// a typed render path and a raw --json path off a single Unary decode into
// map[string]any.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
