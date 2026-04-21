package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// flagRegistrar declares flags on a target FlagSet. Used for Group.Flags
// closures and by WithAncestorFlags to stack ancestor contributions that
// every descendant leaf can pull in.
type flagRegistrar = func(*flag.FlagSet)

// ancestorFlagsKey is the unexported ctx key for the stack of ancestor flag
// registrars accumulated by Group.Run as dispatch descends. Each entry is
// called on a leaf's FlagSet from ApplyAncestorFlags so common flags
// declared on a Group (e.g. --name on a dialect subtree) appear on every
// child without per-leaf duplication.
type ancestorFlagsKey struct{}

// WithAncestorFlags appends reg to the ctx-carried slice of ancestor flag
// registrars and returns the new context. Groups call this during Run so
// child commands inherit the Group's declared flags; the slice preserves
// registration order (outermost ancestor first) so leaf tests can reason
// about precedence by inserting guards.
//
// Per stdlib context.WithValue contract, ctx must not be nil.
func WithAncestorFlags(ctx context.Context, reg func(*flag.FlagSet)) context.Context {
	prior, _ := ctx.Value(ancestorFlagsKey{}).([]flagRegistrar)
	next := make([]flagRegistrar, 0, len(prior)+1)
	next = append(next, prior...)
	next = append(next, reg)
	return context.WithValue(ctx, ancestorFlagsKey{}, next)
}

// ApplyAncestorFlags runs every registered ancestor registrar on fs in the
// order they were appended (outermost first). Leaves call this AFTER
// declaring their own flags so leaf declarations populate fs first and each
// ancestor registrar can Lookup-guard its own additions — stdlib
// flag.FlagSet panics on duplicate declarations, and this ordering makes
// "leaf wins" fall out naturally.
//
// Callers that build ancestor registrars should wrap StringVar/BoolVar in
// the DeclareString / DeclareBool helpers (or equivalent Lookup guards) so
// they're safe when the leaf declared the same name.
func ApplyAncestorFlags(ctx context.Context, fs *flag.FlagSet) {
	regs, _ := ctx.Value(ancestorFlagsKey{}).([]flagRegistrar)
	for _, r := range regs {
		r(fs)
	}
}

// DeclareString is a Lookup-guarded wrapper around fs.StringVar. Ancestor
// Group.Flags closures should use this (rather than raw StringVar) so a
// leaf that already declared the same name isn't clobbered by a duplicate
// declaration — the stdlib flag package panics in that case.
func DeclareString(fs *flag.FlagSet, target *string, name, def, usage string) {
	if fs.Lookup(name) == nil {
		fs.StringVar(target, name, def, usage)
	}
}

// DeclareBool is the bool counterpart to DeclareString. Same guard against
// panicking on duplicate names.
func DeclareBool(fs *flag.FlagSet, target *bool, name string, def bool, usage string) {
	if fs.Lookup(name) == nil {
		fs.BoolVar(target, name, def, usage)
	}
}

// Flagger is an optional opt-in for leaf commands whose help should include
// a flag enumeration that stacks ancestor-declared flags with the leaf's
// own. Leaves that implement Flags(fs) get an automatic Flags: block
// appended to their --help output by dispatchChild; leaves that don't
// implement it keep the current hand-written Help() as their sole source
// of usage text.
type Flagger interface {
	Flags(fs *flag.FlagSet)
}

// globalFlagSpec records a root-level flag name and whether it consumes the
// next token as its value. Keep in sync with ParseGlobal's FlagSet — the
// TestGlobalFlagsRegistrySync regression test asserts they do not drift.
type globalFlagSpec struct {
	name       string
	takesValue bool
}

// globalFlagRegistry is the canonical list of root-level flags StripGlobals
// knows about. ParseGlobal is the source of truth for behavior; this slice
// mirrors that shape so StripGlobals can consume globals at any position in
// argv instead of only at the front (stdlib flag.FlagSet.Parse stops at the
// first positional).
var globalFlagRegistry = []globalFlagSpec{
	{name: "json", takesValue: false},
	{name: "endpoint", takesValue: true},
	{name: "token-file", takesValue: true},
	{name: "profile", takesValue: true},
}

// globalFlagsHelp renders the root-level flags as a `Global Flags:` block for
// both RootHelp and the leaf --help path. One source of truth so every
// surface lists the same flags with the same usage strings.
func globalFlagsHelp() string {
	var b strings.Builder
	fs := flag.NewFlagSet("global", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var g Global
	fs.BoolVar(&g.JSON, "json", false, "emit JSON output")
	fs.StringVar(&g.Endpoint, "endpoint", "", "override API endpoint URL")
	fs.StringVar(&g.TokenFile, "token-file", "", "path to bearer-token file")
	fs.StringVar(&g.Profile, "profile", "", "config profile to use")
	b.WriteString("Global Flags:\n")
	// Gather, sort, then measure for a stable two-column layout.
	names := make([]string, 0, 4)
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	slices.Sort(names)
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		f := fs.Lookup(n)
		fmt.Fprintf(&b, "  --%-*s   %s\n", width, f.Name, f.Usage)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Global holds the root-level flags that apply to every verb. Command
// implementations read it from context via GlobalFrom; ParseGlobal produces
// it from raw argv.
type Global struct {
	JSON      bool
	Endpoint  string
	TokenFile string
	Profile   string
}

// globalKey is the unexported context key for Global so only this package can
// write it, preventing accidental collisions with other packages.
type globalKey struct{}

// WithGlobal returns a child context carrying g. Per stdlib convention ctx
// must be non-nil; a nil parent panics (mirroring context.WithValue).
func WithGlobal(ctx context.Context, g Global) context.Context {
	return context.WithValue(ctx, globalKey{}, g)
}

// GlobalFrom extracts the Global from ctx, or a zero value if absent. Per
// stdlib convention ctx must be non-nil; a nil ctx panics (mirroring
// context.Value semantics).
func GlobalFrom(ctx context.Context) Global {
	g, _ := ctx.Value(globalKey{}).(Global)
	return g
}

// ParseGlobal parses the global flags at the front of args and returns the
// resulting Global along with the remaining args (the verb and its args).
// Parsing stops at the first positional argument or `--`; subcommand flags
// are left to the subcommand itself.
func ParseGlobal(args []string) (Global, []string, error) {
	var g Global
	fs := flag.NewFlagSet("ana", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&g.JSON, "json", false, "emit JSON output")
	fs.StringVar(&g.Endpoint, "endpoint", "", "override API endpoint URL")
	fs.StringVar(&g.TokenFile, "token-file", "", "path to bearer-token file")
	fs.StringVar(&g.Profile, "profile", "", "config profile to use")
	if err := fs.Parse(args); err != nil {
		return Global{}, nil, fmt.Errorf("parse global flags: %w", err)
	}
	return g, fs.Args(), nil
}

// StripGlobals walks args once and splits it into (Global, rest, err). Unlike
// ParseGlobal, which relies on stdlib flag.FlagSet.Parse and stops at the
// first positional, StripGlobals consumes known global flags wherever they
// appear — before, after, or interleaved with the verb path and leaf flags.
// Everything it does not recognise is passed through to rest in original
// order so the leaf's FlagSet still handles unknown-flag errors.
//
// Supported forms per known flag in globalFlagRegistry:
//
//   - `--name` (bool) or `--name=value` / `--name value` (takesValue)
//
// A bare `--` terminator stops global consumption: every remaining token is
// copied verbatim to rest (including the `--` itself), so leaves can still
// use `--` to force positional interpretation of an arg that looks like a
// flag.
//
// Duplicate globals follow stdlib semantics (last wins). Unknown flags are
// left in rest unchanged so the leaf's own FlagSet reports a precise
// `flag provided but not defined: --xyz` at its verb name.
func StripGlobals(args []string) (Global, []string, error) {
	var g Global
	fs := flag.NewFlagSet("ana", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&g.JSON, "json", false, "emit JSON output")
	fs.StringVar(&g.Endpoint, "endpoint", "", "override API endpoint URL")
	fs.StringVar(&g.TokenFile, "token-file", "", "path to bearer-token file")
	fs.StringVar(&g.Profile, "profile", "", "config profile to use")

	rest := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		tok := args[i]
		if tok == "--" {
			// Terminator: preserve it and pass every remaining token through.
			rest = append(rest, args[i:]...)
			break
		}
		name, value, hasEquals, isLong := parseFlagToken(tok)
		if !isLong {
			rest = append(rest, tok)
			i++
			continue
		}
		spec, known := lookupGlobal(name)
		if !known {
			rest = append(rest, tok)
			i++
			continue
		}
		if spec.takesValue {
			if hasEquals {
				if err := fs.Set(name, value); err != nil {
					return Global{}, nil, fmt.Errorf("parse global flags: invalid value %q for -%s: %w", value, name, err)
				}
				i++
				continue
			}
			// Consumes next token as value. Missing next token is a usage error
			// — mirrors stdlib behavior for `flag needs an argument`.
			if i+1 >= len(args) {
				return Global{}, nil, fmt.Errorf("parse global flags: flag needs an argument: -%s", name)
			}
			if err := fs.Set(name, args[i+1]); err != nil {
				return Global{}, nil, fmt.Errorf("parse global flags: invalid value %q for -%s: %w", args[i+1], name, err)
			}
			i += 2
			continue
		}
		// Bool-valued global: `--name` sets true; `--name=false` respects the
		// literal value. A bare `--name` with no `=` is the common path and
		// always sets true.
		raw := "true"
		if hasEquals {
			raw = value
		}
		if err := fs.Set(name, raw); err != nil {
			return Global{}, nil, fmt.Errorf("parse global flags: invalid value %q for -%s: %w", raw, name, err)
		}
		i++
	}
	return g, rest, nil
}

// parseFlagToken classifies a token as a long-form flag (`--name` or
// `--name=value`) and returns its components. isLong is false for anything
// that doesn't start with `--` or for the bare `--` terminator — both are
// passed through to rest untouched.
func parseFlagToken(tok string) (name, value string, hasEquals, isLong bool) {
	if len(tok) < 3 || !strings.HasPrefix(tok, "--") {
		return "", "", false, false
	}
	body := tok[2:]
	if eq := strings.IndexByte(body, '='); eq >= 0 {
		return body[:eq], body[eq+1:], true, true
	}
	return body, "", false, true
}

// lookupGlobal reports whether name is one of the recognised global flags
// and, if so, whether it consumes a value token.
func lookupGlobal(name string) (globalFlagSpec, bool) {
	for _, spec := range globalFlagRegistry {
		if spec.name == name {
			return spec, true
		}
	}
	return globalFlagSpec{}, false
}
