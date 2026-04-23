// Package api provides the `ana api` verb — an authenticated raw-JSON
// passthrough over the shared transport client. Dispatches to a single leaf.
// Two path forms:
//
//   - Leading slash → sent verbatim (REST e.g. `/v1/...`, or a pre-resolved
//     RPC path e.g. `/rpc/public/<service>/<Method>`).
//   - No leading slash → treated as a fully-qualified Connect-RPC short form
//     (e.g. `textql.rpc.public.auth.PublicAuthService/GetOrganization`) and
//     prefixed with `/rpc/public/`.
//
// Like every other verb package, api never imports internal/transport — the
// caller adapts its transport client to the narrow Deps.DoRaw field.
package api

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// connectRPCPrefix is the path prefix applied to Connect-RPC short-form paths
// (those without a leading slash). Matches what every typed verb hard-codes.
const connectRPCPrefix = "/rpc/public/"

// Deps is the injection boundary. A real wiring layer adapts
// transport.Client.DoRaw; tests pass fakes that record (method, path, body)
// so assertions can inspect the outbound request and the returned response.
type Deps struct {
	DoRaw func(ctx context.Context, method, path string, body []byte) (int, []byte, error)
}

// New returns the `api` verb as a single leaf command. Unlike other verb
// packages this is not a *cli.Group — there are no subcommands, just a path
// positional.
func New(deps Deps) cli.Command {
	return &callCmd{deps: deps}
}
