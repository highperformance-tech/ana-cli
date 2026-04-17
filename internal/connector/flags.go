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

// parseFlags invokes fs.Parse and wraps any error with cli.ErrUsage. Keeps
// every command's "return parseFlags(...)" line a single line.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
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
