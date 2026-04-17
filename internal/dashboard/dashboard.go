// Package dashboard provides the `ana dashboard` verb tree: list, folders,
// get, spawn, health. Like its sibling command packages it is pure dispatch
// glue around an injected Unary RPC call so the package never imports
// internal/transport or internal/config — callers adapt their transport
// client to the narrow Deps contract declared here.
package dashboard

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/textql/ana-cli/internal/cli"
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

// newFlagSet returns a FlagSet configured for leaf commands: continue-on-error
// so we can wrap parse failures as usage errors, and silenced output so each
// command's own Help() is the sole source of usage text.
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
// it via errors.Is.
func usageErrf(format string, a ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), cli.ErrUsage)
}

// requireID pulls a single UUID-style positional from args. It does not
// validate UUID format because the API tolerates any non-empty string and
// returns a clear error if the shape is wrong.
func requireID(verb string, args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", usageErrf("%s: <id> positional argument required", verb)
	}
	return args[0], nil
}

// writeJSON pretty-prints v to w with a 2-space indent and trailing newline.
// Used by every `--json` branch so output is uniform across verbs.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal round-trips src through JSON into dst, letting commands have both
// a typed render path and a raw --json path off a single Unary decode into
// map[string]any.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
