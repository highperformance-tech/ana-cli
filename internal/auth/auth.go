// Package auth provides the `ana auth` verb tree: login, logout, whoami, and
// the nested `keys` and `service-accounts` groups. It is pure dispatch logic
// composed around an injected transport/config boundary (see Deps), so it
// never imports internal/transport or internal/config directly — those
// packages' concrete types merely satisfy the narrow interfaces this package
// defines.
package auth

import (
	"context"
	"errors"

	"github.com/textql/ana-cli/internal/cli"
)

// DefaultEndpoint is used when neither the --endpoint global nor a persisted
// config value is present.
const DefaultEndpoint = "https://app.textql.com"

// ErrNotLoggedIn is returned by commands that require an auth token when the
// loaded config has none. cli.ExitCode maps this to exit code 2 (generic
// error). Callers can assert on it via errors.Is.
var ErrNotLoggedIn = errors.New("not logged in")

// Config is the persisted subset of configuration this package reads/writes.
// The concrete type in internal/config has additional fields, but auth only
// needs endpoint + token, and this local shape keeps the contract narrow.
type Config struct {
	Endpoint string
	Token    string
}

// Deps is the injected boundary. A real wiring layer adapts transport.Client
// and config.Load/Save to these function fields; tests pass fakes.
//
// Unary performs one Connect-RPC call: the request value is JSON-encoded,
// the response pointer is JSON-decoded. If the returned error satisfies the
// unexported authSignaler interface with IsAuthError() == true, or has an
// error string containing "unauthenticated", it is translated into this
// package's own auth-error wrapper so cli.ExitCode can map it to exit 3.
type Deps struct {
	Unary      func(ctx context.Context, path string, req, resp any) error
	LoadCfg    func() (Config, error)
	SaveCfg    func(Config) error
	ConfigPath func() (string, error)
}

// New returns the auth verb group. Its children are the leaf commands and
// two nested groups (`keys`, `service-accounts`). The returned *cli.Group is
// safe to register under any name in the root verb table.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Manage authentication, API keys, and service accounts.",
		Children: map[string]cli.Command{
			"login":            &loginCmd{deps: deps},
			"logout":           &logoutCmd{deps: deps},
			"whoami":           &whoamiCmd{deps: deps},
			"keys":             newKeysGroup(deps),
			"service-accounts": newServiceAccountsGroup(deps),
		},
	}
}
