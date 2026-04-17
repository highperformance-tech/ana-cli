// Package connector provides the `ana connector` verb tree: list, get, create,
// update, delete, test, tables, examples. It is pure dispatch glue around an
// injected Unary RPC call (see Deps) so tests pass a fake and the package
// never imports internal/transport or internal/config directly.
package connector

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// servicePath is the Connect-RPC service prefix every connector endpoint lives
// under. Centralised so tests can assert on the full path without drift.
const servicePath = "/rpc/public/textql.rpc.public.connector.ConnectorService"

// Deps is the narrow injection boundary. Unary JSON-encodes req, POSTs it to
// path, and JSON-decodes the response into *resp. A concrete wiring layer
// adapts transport.Client to this function field; tests pass a recording fake.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `connector` verb group. The returned *cli.Group is safe to
// register under any name in the root verb table.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage data connectors (list/get/create/update/delete/test/tables/examples).",
		Children: map[string]cli.Command{
			"list":     &listCmd{deps: deps},
			"get":      &getCmd{deps: deps},
			"create":   &createCmd{deps: deps},
			"update":   &updateCmd{deps: deps},
			"delete":   &deleteCmd{deps: deps},
			"test":     &testCmd{deps: deps},
			"tables":   &tablesCmd{deps: deps},
			"examples": &examplesCmd{deps: deps},
		},
	}
}
