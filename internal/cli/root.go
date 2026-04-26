package cli

import (
	"context"
	"strings"
)

// Global holds the root-level flags that apply to every verb. Command
// implementations read it from context via GlobalFrom; the resolver
// populates it from the parsed merged FlagSet via globalFromFlagSet.
//
// Names match the root group's persistent Flags closure declared in
// cmd/ana/main.go: --json, --endpoint, --token-file, --profile.
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

// parseFlagToken classifies a token as a long-form flag (`--name`,
// `-name`, `--name=value`, or `-name=value`) and returns its components.
// Both single- and double-dash spellings are accepted per stdlib
// `flag.FlagSet.Parse` docs ("One or two dashes may be used; they are
// equivalent"). isLong is false for non-flag tokens, the bare `--`
// terminator, or a bare `-` — all are passed through to the leaf untouched.
func parseFlagToken(tok string) (name, value string, hasEquals, isLong bool) {
	if len(tok) < 2 || tok[0] != '-' {
		return "", "", false, false
	}
	body := tok[1:]
	if body[0] == '-' {
		// `--` terminator or `--name` — strip the second dash. A bare `--`
		// has body=="-" here which we reject below (len check).
		body = body[1:]
	}
	if body == "" || body == "-" {
		return "", "", false, false
	}
	if eq := strings.IndexByte(body, '='); eq >= 0 {
		return body[:eq], body[eq+1:], true, true
	}
	return body, "", false, true
}
