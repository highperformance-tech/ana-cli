package auth

import (
	"errors"
	"strings"
)

// authSignaler is the narrow local interface the transport's *Error type
// happens to satisfy (see internal/transport.Error.IsAuth). We re-declare it
// here to avoid an import cycle and keep the package dependency-free.
type authSignaler interface {
	IsAuthError() bool
}

// authErr wraps an underlying transport error and re-exposes the auth signal
// so cli.ExitCode (which looks for IsAuthError via errors.As) can map it to
// exit code 3 without having any knowledge of this package.
type authErr struct {
	wrapped error
}

// Error delegates to the wrapped error's message so end-user output stays
// unchanged — the wrapping is a signal, not a cosmetic change.
func (e *authErr) Error() string { return e.wrapped.Error() }

// Unwrap enables errors.Is/As traversal into the underlying error.
func (e *authErr) Unwrap() error { return e.wrapped }

// IsAuthError signals to cli.ExitCode that this is a 401/unauthenticated
// failure (always true — construction implies the signal).
func (e *authErr) IsAuthError() bool { return true }

// translateErr returns err as-is unless it looks like an auth failure; in
// that case it returns a *authErr so the signal flows through to the root.
// Two detection paths: explicit interface (transport.Error.IsAuth) and a
// string match on "unauthenticated" for servers that only return the code.
func translateErr(err error) error {
	if err == nil {
		return nil
	}
	// If the underlying error already exposes IsAuthError via its own
	// interface, honor that first — this is the transport.Error path.
	var s authSignaler
	if errors.As(err, &s) && s.IsAuthError() {
		return &authErr{wrapped: err}
	}
	// Fallback: servers sometimes surface the Connect "unauthenticated" code
	// only in the error message. We match case-insensitively to be lenient.
	if strings.Contains(strings.ToLower(err.Error()), "unauthenticated") {
		return &authErr{wrapped: err}
	}
	return err
}
