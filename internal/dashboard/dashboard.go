// Package dashboard provides the `ana dashboard` verb tree: list, folders,
// get, spawn, health. Like its sibling command packages it is pure dispatch
// glue around an injected Unary RPC call so the package never imports
// internal/transport or internal/config — callers adapt their transport
// client to the narrow Deps contract declared here.
package dashboard

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// servicePath is the Connect-RPC service prefix every dashboard endpoint
// lives under. Centralised so tests can assert on the full path without
// drift between verbs.
const servicePath = "/rpc/public/textql.rpc.public.dashboard.DashboardService"

// Deps is the narrow injection boundary. Unary JSON-encodes req, POSTs it to
// path, and JSON-decodes the response into *resp. A concrete wiring layer
// adapts transport.Client to this function field; tests pass a recording fake.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `dashboard` verb group. The returned *cli.Group is safe to
// register under any name in the root verb table.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect and control TextQL dashboards (list/get/spawn/health).",
		Children: map[string]cli.Command{
			"list":    &listCmd{deps: deps},
			"folders": newFoldersGroup(deps),
			"get":     &getCmd{deps: deps},
			"spawn":   &spawnCmd{deps: deps},
			"health":  &healthCmd{deps: deps},
		},
	}
}
