package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// NewFlagSet returns a *flag.FlagSet configured the way every leaf verb wants
// it: ContinueOnError so we can wrap parse failures as usage errors, and
// output silenced (io.Discard) so each command's own Help() is the sole
// source of usage text. Currently duplicated in every verb package — Phases
// 1–10 of the shared-cli-kit refactor switch them over to this helper.
func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// UsageErrf builds a user-facing error that wraps ErrUsage so the root
// dispatcher maps it to exit code 1 via errors.Is. Use this anywhere a verb
// detects a missing/invalid arg or other shape problem the caller could fix
// by re-running with different inputs.
func UsageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), ErrUsage)
}

// WriteJSON pretty-prints v to w with a 2-space indent and trailing newline,
// matching the convention used across every --json branch in the CLI. A
// single helper keeps output byte-identical between verbs.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// Remarshal round-trips src through JSON into dst, letting commands have one
// Unary decode into map[string]any and still derive a typed view for table
// rendering without a second RPC.
func Remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// RequireStringID extracts a non-empty positional <id> from args[0]. An
// empty or whitespace-only first arg (or a missing arg entirely) is rejected
// with a usage error. The strictest pre-refactor variant (dashboard's
// strings.TrimSpace check) is used here so all verb packages converge on
// identical behaviour after migration.
func RequireStringID(verb string, args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", UsageErrf("%s: <id> positional argument required", verb)
	}
	return args[0], nil
}

// RequireIntID extracts an integer positional <id> from args[0]. Missing or
// empty input returns the same usage error as RequireStringID; a non-numeric
// input returns a usage error that quotes the underlying strconv error.
// Behaviour matches the pre-refactor connector.atoiID with the addition of
// the args-slice indirection.
func RequireIntID(verb string, args []string) (int, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return 0, UsageErrf("%s: <id> positional argument required", verb)
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, UsageErrf("%s: <id> must be an integer: %v", verb, err)
	}
	return id, nil
}

// ReadToken consumes stdin and returns a trimmed token. With tokenStdin=true
// the whole stream is consumed; otherwise a single newline-terminated line is
// read. Whitespace is trimmed in both modes so common pipe quirks (trailing
// newline from `echo`) don't poison the saved value. Centralised here because
// auth login and profile add share this exact behaviour.
func ReadToken(r io.Reader, tokenStdin bool) (string, error) {
	if r == nil {
		return "", fmt.Errorf("stdin is nil")
	}
	if tokenStdin {
		b, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	// Boost the buffer so unusually long tokens (JWTs etc.) fit in one line.
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scan.Scan() {
		return strings.TrimSpace(scan.Text()), nil
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return "", nil
}

// NewTableWriter returns a *tabwriter.Writer configured the way every verb's
// list/show table wants it: no min-width, no tab-stop, two-space padding.
// Callers must call Flush() (or defer it) once they finish writing rows.
func NewTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// RenderTwoCol prints top-level scalar fields then any nested map fields
// (e.g. postgresMetadata) as an indented sub-block. Keys are sorted so the
// output is stable across runs for snapshot-style tests. Output is
// byte-identical to the pre-refactor connector/get.go::renderTwoCol.
func RenderTwoCol(w io.Writer, m map[string]any) error {
	tw := NewTableWriter(w)
	scalarKeys := make([]string, 0, len(m))
	nestedKeys := make([]string, 0)
	for k, v := range m {
		if _, ok := v.(map[string]any); ok {
			nestedKeys = append(nestedKeys, k)
			continue
		}
		scalarKeys = append(scalarKeys, k)
	}
	sort.Strings(scalarKeys)
	sort.Strings(nestedKeys)
	for _, k := range scalarKeys {
		fmt.Fprintf(tw, "%s:\t%v\n", k, m[k])
	}
	for _, k := range nestedKeys {
		fmt.Fprintf(tw, "%s:\t\n", k)
		sub := m[k].(map[string]any)
		subKeys := make([]string, 0, len(sub))
		for sk := range sub {
			subKeys = append(subKeys, sk)
		}
		sort.Strings(subKeys)
		for _, sk := range subKeys {
			fmt.Fprintf(tw, "  %s:\t%v\n", sk, sub[sk])
		}
	}
	return tw.Flush()
}
