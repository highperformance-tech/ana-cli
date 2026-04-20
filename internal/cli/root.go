package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// Global holds the root-level flags that apply to every verb. Command
// implementations read it from context via GlobalFrom; ParseGlobal produces
// it from raw argv.
type Global struct {
	JSON      bool
	Endpoint  string
	TokenFile string
	Profile   string
	Verbose   bool
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
	fs.BoolVar(&g.Verbose, "verbose", false, "verbose logging")
	fs.BoolVar(&g.Verbose, "v", false, "verbose logging (shorthand)")
	if err := fs.Parse(args); err != nil {
		return Global{}, nil, fmt.Errorf("parse global flags: %w", err)
	}
	return g, fs.Args(), nil
}
