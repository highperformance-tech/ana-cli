// Package org provides the `ana org` verb tree: show, members, roles, and
// permissions. Like internal/auth it is pure dispatch logic composed around an
// injected Unary function, so it never imports internal/transport — callers
// adapt their transport client to the narrow Deps contract declared here.
package org

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// Deps is the injected boundary. A real wiring layer adapts transport.Client
// to the single function field; tests pass fakes that record the path and
// request payload so assertions can check the wire-level field names.
//
// Unary performs one Connect-RPC call: the request value is JSON-encoded,
// the response pointer is JSON-decoded.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `org` verb group. Its children are the leaf `show` command
// and three nested groups (`members`, `roles`, `permissions`) each with a
// single `list` child. The returned *cli.Group is safe to register under any
// name in the root verb table.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect the current organization, members, roles, and permissions.",
		Children: map[string]cli.Command{
			"show":        &showCmd{deps: deps},
			"list":        &listCmd{deps: deps},
			"members":     newMembersGroup(deps),
			"roles":       newRolesGroup(deps),
			"permissions": newPermissionsGroup(deps),
		},
	}
}
