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

// defaultEndpoint is the fallback base URL used in human-readable success
// notes when Deps.Endpoint is empty (e.g. tests that wire a bare Deps).
// Mirrors config.DefaultEndpoint without importing it — verb packages do not
// depend on internal/config.
const defaultEndpoint = "https://app.textql.com"

// Deps is the narrow injection boundary. Unary JSON-encodes req, POSTs it to
// path, and JSON-decodes the response into *resp. A concrete wiring layer
// adapts transport.Client to this function field; tests pass a recording fake.
//
// Endpoint is a closure that returns the resolved API base URL (after
// --endpoint / profile / env precedence), used by OAuth leaves whose success
// notes direct users at the correct TextQL web app to complete the browser
// handshake. The closure form lets the wiring layer defer config-load until
// the OAuth verb actually runs (so non-OAuth verbs never trigger it). A nil
// closure or an empty return value falls back to defaultEndpoint.
type Deps struct {
	Unary    func(ctx context.Context, path string, req, resp any) error
	Endpoint func() string
}

// resolveEndpoint returns d.Endpoint() when non-empty, else defaultEndpoint.
// OAuth success notes call this so self-hosted and non-prod profiles point
// users at the right web app instead of always echoing app.textql.com.
func (d Deps) resolveEndpoint() string {
	if d.Endpoint != nil {
		if e := d.Endpoint(); e != "" {
			return e
		}
	}
	return defaultEndpoint
}

// New returns the `connector` verb group. The returned *cli.Group is safe to
// register under any name in the root verb table.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage data connectors (list/get/create/update/delete/test/tables/examples).",
		Children: map[string]cli.Command{
			"list":     &listCmd{deps: deps},
			"get":      &getCmd{deps: deps},
			"create":   newCreateGroup(deps),
			"update":   &updateCmd{deps: deps},
			"delete":   &deleteCmd{deps: deps},
			"test":     &testCmd{deps: deps},
			"tables":   &tablesCmd{deps: deps},
			"examples": &examplesCmd{deps: deps},
		},
	}
}
