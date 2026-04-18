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
