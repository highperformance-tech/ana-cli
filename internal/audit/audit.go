// Package audit provides the `ana audit` verb tree: tail. Like the other
// verb packages it avoids importing internal/transport and internal/config —
// callers inject a narrow Deps struct that adapts a real transport client to
// a single Unary function field plus an injectable clock for deterministic
// `--since` tests.
package audit

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// auditServicePath is the Connect-RPC prefix the AuditLogService uses. The
// underlying proto package is `textql.rpc.public.audit_log` (note the
// underscore); the catalog filename uses the same form.
const auditServicePath = "/rpc/public/textql.rpc.public.audit_log.AuditLogService"

// Deps is the injection boundary for the audit package.
//
// Unary JSON-encodes req, POSTs it to path, and JSON-decodes into *resp. A
// real wiring layer adapts transport.Client to this signature.
//
// Now is the clock used to compute `--since` timestamps. Defaulting Now to
// time.Now is the caller's responsibility; New does it for you.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
	Now   func() time.Time
}

// New returns the `audit` verb group. A zero Now defaults to time.Now so the
// common wiring path does not need to remember it; tests pass a fake clock to
// exercise the `--since` branch deterministically.
func New(deps Deps) *cli.Group {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &cli.Group{
		Summary: "Inspect audit logs: tail.",
		Children: map[string]cli.Command{
			"tail": &tailCmd{deps: deps},
		},
	}
}

// newFlagSet returns a FlagSet with ContinueOnError + silenced output so each
// command's own Help() is the single source of usage text.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags delegates to cli.ParseFlags so positional args can be
// interleaved with flags without silently dropping trailing flags.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return cli.ParseFlags(fs, args)
}

// usageErrf emits a user-facing usage error.
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
