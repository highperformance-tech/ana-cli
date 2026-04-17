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
func (e *Error) Error() string {
	return fmt.Sprintf("transport: %d %s: %s", e.HTTPStatus, e.Code, e.Message)
}

// IsAuth reports whether err (or anything it wraps) is a transport.Error that
// represents an authentication failure — either HTTP 401 or a Connect code of
// "unauthenticated". It returns false for nil and for non-transport errors.
func IsAuth(err error) bool {
	var te *Error
	if !errors.As(err, &te) {
		return false
	}
	return te.HTTPStatus == 401 || te.Code == "unauthenticated"
}
