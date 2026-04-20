package cli

import (
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
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

// RequireFlags returns a UsageErr listing any flag from names that was not
// explicitly set on fs. Built on FlagWasSet, so "explicit zero value"
// (e.g. --port 0) still counts as supplied — callers that care about the
// value's content validate it themselves. The verb prefix is prepended so
// the error reads `verb: missing required flags: --a, --b`.
func RequireFlags(fs *flag.FlagSet, verb string, names ...string) error {
	var missing []string
	for _, n := range names {
		if !FlagWasSet(fs, n) {
			missing = append(missing, "--"+n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return UsageErrf("%s: missing required flags: %s", verb, strings.Join(missing, ", "))
}

// EnumFlag returns a flag.Value that validates against a fixed allow-list at
// parse time: unknown values yield a UsageErr before the verb body runs, so
// downstream code can trust *target. The allowed values show up in the Set
// error, which fs.Parse wraps as `invalid value "X" for flag -Y: allowed: ...`.
// Pairs naturally with RequireFlags when the flag is also mandatory.
func EnumFlag(target *string, allowed []string) flag.Value {
	return &enumFlag{target: target, allowed: allowed}
}

type enumFlag struct {
	target  *string
	allowed []string
}

func (e *enumFlag) String() string {
	if e.target == nil {
		return ""
	}
	return *e.target
}

func (e *enumFlag) Set(s string) error {
	if !slices.Contains(e.allowed, s) {
		return UsageErrf("allowed: %s", strings.Join(e.allowed, ", "))
	}
	*e.target = s
	return nil
}

// IntListFlag returns a flag.Value that parses a separator-delimited list of
// ints into *target (whitespace around each entry is tolerated). Empty input
// and non-integer tokens produce a UsageErr; on success *target is guaranteed
// non-empty.
func IntListFlag(target *[]int, sep string) flag.Value {
	return &intListFlag{target: target, sep: sep}
}

type intListFlag struct {
	target *[]int
	sep    string
}

func (l *intListFlag) String() string {
	if l.target == nil || len(*l.target) == 0 {
		return ""
	}
	parts := make([]string, len(*l.target))
	for i, n := range *l.target {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, l.sep)
}

func (l *intListFlag) Set(s string) error {
	trim := strings.TrimSpace(s)
	if trim == "" {
		return UsageErrf("at least one id required")
	}
	parts := strings.Split(trim, l.sep)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return UsageErrf("empty id in list")
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return UsageErrf("%q is not an integer", p)
		}
		out = append(out, n)
	}
	*l.target = out
	return nil
}

// SinceFlag returns a flag.Value that accepts either a non-negative duration
// (e.g. "1h", "24h") — interpreted as `now() - dur` — or an absolute RFC3339
// timestamp, and stores the UTC-normalised result in *target. The injected
// now lets tests fix the clock so --since assertions are deterministic.
// Negative durations are rejected rather than silently producing a future
// timestamp, which would mask operator typos.
func SinceFlag(target *time.Time, now func() time.Time) flag.Value {
	return &sinceFlag{target: target, now: now}
}

type sinceFlag struct {
	target *time.Time
	now    func() time.Time
}

func (s *sinceFlag) String() string {
	if s.target == nil || s.target.IsZero() {
		return ""
	}
	return s.target.UTC().Format(time.RFC3339)
}

func (s *sinceFlag) Set(raw string) error {
	if dur, err := time.ParseDuration(raw); err == nil {
		if dur < 0 {
			return UsageErrf("duration must be >= 0")
		}
		*s.target = s.now().Add(-dur).UTC()
		return nil
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		*s.target = ts.UTC()
		return nil
	}
	return UsageErrf("expected duration like 1h or RFC3339 timestamp")
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
