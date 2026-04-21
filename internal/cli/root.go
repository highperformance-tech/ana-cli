package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	next = append(next, flagRegistrar(reg))
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
