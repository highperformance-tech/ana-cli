// Package profile provides the `ana profile` verb tree: list, add, use,
// remove, and show. API keys on app.textql.com are org-scoped, so users who
// work across multiple TextQL orgs keep one profile per org and flip between
// them with `ana profile use`.
//
// Unlike internal/auth — which declares its own narrow Config type so the
// auth package never depends on internal/config — the profile verb is
// INHERENTLY about the whole config structure: listing profiles, switching
// the active pointer, etc. Deps therefore speaks config.Config directly and
// this package imports internal/config. That is the intentional design, not
// an accidental coupling.
package profile

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/config"
)

// Deps is the injected boundary. The wiring layer in cmd/ana supplies
// closures backed by config.Load/Save and config.DefaultPath; tests supply
// the same closures pointed at a t.TempDir() file so the actual round-trip
// is exercised end-to-end.
type Deps struct {
	LoadCfg    func() (config.Config, error)
	SaveCfg    func(config.Config) error
	ConfigPath func() (string, error)
}

// New returns the `profile` verb group.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage named config profiles (one per TextQL org).",
		Children: map[string]cli.Command{
			"list":   &listCmd{deps: deps},
			"add":    &addCmd{deps: deps},
			"use":    &useCmd{deps: deps},
			"remove": &removeCmd{deps: deps},
			"show":   &showCmd{deps: deps},
		},
	}
}

// newFlagSet mirrors the idiom in internal/org: continue-on-error parsing
// and silenced output so each command owns its own Help() rendering.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags delegates to cli.ParseFlags so positional args can be
// interleaved with flags without silently dropping trailing flags.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return cli.ParseFlags(fs, args)
}

// usageErrf produces a cli.ErrUsage-wrapped error with a formatted message.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// writeJSON indents v to w with the same 2-space style the other verbs use.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}
