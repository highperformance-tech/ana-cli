// Package audit provides the `ana audit` verb tree: tail. Like the other
// verb packages it avoids importing internal/transport and internal/config —
// callers inject a narrow Deps struct that adapts a real transport client to
// a single Unary function field plus an injectable clock for deterministic
// `--since` tests.
package audit

import (
	"context"
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
