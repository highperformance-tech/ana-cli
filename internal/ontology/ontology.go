// Package ontology provides the `ana ontology` verb tree: list and get. Like
// the other verb packages it avoids importing internal/transport and
// internal/config — callers inject a narrow Deps struct that adapts a real
// transport client to a single Unary function field.
package ontology

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/textql/ana-cli/internal/cli"
)

// ontologyServicePath is the Connect-RPC prefix every OntologyService endpoint
// uses. Centralised so tests can assert against the full path and refactors
// stay mechanical.
const ontologyServicePath = "/rpc/public/textql.rpc.public.ontology.OntologyService"

// Deps is the injection boundary for the ontology package.
//
// Unary JSON-encodes req, POSTs it to path, and JSON-decodes into *resp. A
// real wiring layer adapts transport.Client to this signature; tests pass
// fakes that record the path and payload for wire-level assertions.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `ontology` verb group. Safe to register under any name in
// the root verb table — the group holds no state of its own, only a handful
// of *<verb>Cmd structs that capture the shared Deps.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect ontologies: list, get.",
		Children: map[string]cli.Command{
			"list": &listCmd{deps: deps},
			"get":  &getCmd{deps: deps},
		},
	}
}

// newFlagSet returns a FlagSet the way every leaf command wants it: continue
// on parse error (no os.Exit), all output silenced so each command's own
// Help() is the single source of usage text.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags parses args into fs and wraps any error with cli.ErrUsage so the
// root dispatcher maps the failure to exit code 1.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%s: %w: %w", fs.Name(), err, cli.ErrUsage)
	}
	return nil
}

// usageErrf is the canonical way to emit a user-facing usage error.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// writeJSON indents a value to w with the 2-space convention used across the
// CLI.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal round-trips src through JSON into dst.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// requirePositionalID extracts a non-empty positional <id> from the first arg,
// returning a usage error otherwise. Ontology IDs are integer-valued on the
// wire; the wire shape is produced by the caller (the positional is kept as a
// raw string here for flexibility).
func requirePositionalID(verb string, args []string) (string, error) {
	if len(args) == 0 || args[0] == "" {
		return "", usageErrf("%s: <id> positional argument required", verb)
	}
	return args[0], nil
}
