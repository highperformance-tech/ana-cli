// Package playbook provides the `ana playbook` verb tree: list, get, reports,
// and lineage. Like the other verb packages it avoids importing
// internal/transport and internal/config — callers inject a narrow Deps struct
// that adapts a real transport client to a single Unary function field.
package playbook

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// playbookServicePath is the Connect-RPC prefix every PlaybookService endpoint
// uses. Centralised so tests can assert against the full path and refactors
// stay mechanical.
const playbookServicePath = "/rpc/public/textql.rpc.public.playbook.PlaybookService"

// Deps is the injection boundary for the playbook package.
//
// Unary JSON-encodes req, POSTs it to path, and JSON-decodes into *resp. A
// real wiring layer adapts transport.Client to this signature; tests pass
// fakes that record the path and payload for wire-level assertions.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `playbook` verb group. Safe to register under any name in
// the root verb table — the group holds no state of its own, only a handful
// of *<verb>Cmd structs that capture the shared Deps.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect playbooks: list, get, reports, lineage.",
		Children: map[string]cli.Command{
			"list":    &listCmd{deps: deps},
			"get":     &getCmd{deps: deps},
			"reports": &reportsCmd{deps: deps},
			"lineage": &lineageCmd{deps: deps},
		},
	}
}
