// Package org provides the `ana org` verb tree: show, members, roles, and
// permissions. Like internal/auth it is pure dispatch logic composed around an
// injected Unary function, so it never imports internal/transport — callers
// adapt their transport client to the narrow Deps contract declared here.
package org

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

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
			"members":     newMembersGroup(deps),
			"roles":       newRolesGroup(deps),
			"permissions": newPermissionsGroup(deps),
		},
	}
}

// newFlagSet builds a FlagSet configured the way every leaf command wants:
// continue-on-error parsing (no os.Exit) and all output suppressed (each
// command prints its own Help() via the Command interface).
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags invokes fs.Parse and wraps any error with cli.ErrUsage so the
// root dispatcher maps the failure to exit code 1.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
	}
	return nil
}

// usageErrf wraps cli.ErrUsage with a formatted message so callers can detect
// it via errors.Is. Used for positional-arg violations and the like.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// writeJSON indents a value to w using the stdlib encoder. Centralised so
// every --json branch formats identically (2-space indent, trailing newline).
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal decodes src (usually a generic map from a first pass) into dst.
// Used so commands can have both a typed path (for table rendering) and a raw
// --json path without two separate RPC calls.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
