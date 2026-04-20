// Package transport provides a Connect-RPC-over-JSON client used by ana-cli
// to talk to the TextQL backend. Only the Go standard library is used.
package transport

import (
	"errors"
	"fmt"
)

// Error is the concrete error type returned from transport calls when an HTTP
// request completes but the server indicates failure (non-2xx) or when the
// server streams a trailer frame containing an error payload. It captures both
// the HTTP-level status and the decoded Connect/Oathkeeper error envelope.
type Error struct {
	// HTTPStatus is the HTTP status code from the response. Zero when the
	// Error originates from a stream trailer (no HTTP status associated).
	HTTPStatus int
	// Code is the Connect (or Oathkeeper nested) error code string, e.g.
	// "invalid_argument" or "unauthenticated". Empty when the server did not
	// return a recognized envelope.
	Code string
	// Message is the human-readable message from the error envelope.
	Message string
	// Raw is the raw response body bytes, populated when the envelope could
	// not be decoded so callers can still surface something useful.
	Raw []byte
}

// Error implements the standard error interface. The format is stable and
// intended for end-user surfacing: `transport: <status> <code>: <message>`.
// When HTTPStatus is zero — the Error originated from a stream trailer and
// has no HTTP status to report — the leading `0 : ` prefix is elided so the
// rendered string stays readable, e.g. `transport: <message>`. A nil receiver
// renders as `transport: <nil>` so callers that lifted a typed-nil *Error into
// an `error` interface (e.g. via errors.As on a chain holding one) never panic
// dereferencing the fields below.
func (e *Error) Error() string {
	if e == nil {
		return "transport: <nil>"
	}
	if e.HTTPStatus == 0 {
		return fmt.Sprintf("transport: %s", e.Message)
	}
	return fmt.Sprintf("transport: %d %s: %s", e.HTTPStatus, e.Code, e.Message)
}

// IsAuth reports whether err (or anything it wraps) is a transport.Error that
// represents an authentication failure — either HTTP 401 or a Connect code of
// "unauthenticated". It returns false for nil and for non-transport errors.
// Delegates to (*Error).IsAuthError so the classification rule lives in one
// place. The explicit nil check guards against the typed-nil case where
// errors.As succeeds on an interface holding an (*Error)(nil) — the method
// dispatch below would otherwise panic dereferencing a nil receiver.
func IsAuth(err error) bool {
	var te *Error
	if !errors.As(err, &te) || te == nil {
		return false
	}
	return te.IsAuthError()
}

// IsAuthError lets *Error satisfy the unexported `IsAuthError() bool`
// interface that `cli.ExitCode` and `auth.translateErr` use to classify auth
// failures without string matching. A nil receiver returns false so callers
// that pluck a *Error out of a wrapped chain via errors.As — which can land
// on (*Error)(nil) when an interface holds a typed-nil — never panic.
func (e *Error) IsAuthError() bool {
	if e == nil {
		return false
	}
	return e.HTTPStatus == 401 || e.Code == "unauthenticated"
}
