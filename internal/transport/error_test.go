package transport

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "with http status",
			err:  &Error{HTTPStatus: 418, Code: "teapot", Message: "short and stout"},
			want: "transport: 418 teapot: short and stout",
		},
		{
			// A stream-trailer-origin Error has no HTTP status; rendering
			// `0 : msg` would look like a bug to users, so the formatter
			// elides the missing fields.
			name: "stream trailer with no http status",
			err:  &Error{Message: "connection reset"},
			want: "transport: connection reset",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// typedNilError returns an `error` interface whose dynamic value is a typed-nil
// *Error. The indirection through a function return is required: a direct
// `var err error = (*Error)(nil)` trips staticcheck SA4023 ("the comparison is
// never true") because the checker knows the concrete type and collapses the
// interface-level nil check. Going through a function return hides the concrete
// type so the runtime typed-nil vs interface-nil distinction is the one
// actually exercised.
func typedNilError() error {
	var te *Error
	return te
}

// TestErrorStringNilReceiver guards the typed-nil *Error case for the standard
// error-interface method. A (*Error)(nil) lifted into an `error` interface
// (e.g. via errors.As landing on a chain that held one) must render cleanly
// rather than panicking when a caller prints or wraps it.
func TestErrorStringNilReceiver(t *testing.T) {
	t.Parallel()
	const want = "transport: <nil>"
	t.Run("direct call on typed-nil", func(t *testing.T) {
		t.Parallel()
		var e *Error // typed nil
		if got := e.Error(); got != want {
			t.Fatalf("(*Error)(nil).Error() = %q, want %q", got, want)
		}
	})
	t.Run("via error interface", func(t *testing.T) {
		t.Parallel()
		// An `error` interface whose dynamic value is a (*Error)(nil) — the
		// exact trap that surfaces when callers treat a `*Error` return as an
		// `error` without a nil guard.
		err := typedNilError()
		if err == nil {
			t.Fatalf("test setup invariant violated: typed-nil *Error should lift into a non-nil error interface")
		}
		var got string
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("(*Error)(nil).Error() panicked via interface: %v", r)
				}
			}()
			got = err.Error()
		}()
		if got != want {
			t.Fatalf("err.Error() = %q, want %q", got, want)
		}
	})
}

func TestIsAuth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"401 status", &Error{HTTPStatus: 401}, true},
		{"unauthenticated code", &Error{Code: "unauthenticated"}, true},
		{"other code", &Error{Code: "invalid_argument", HTTPStatus: 400}, false},
		{"wrapped 401", fmt.Errorf("outer: %w", &Error{HTTPStatus: 401}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAuth(tc.err); got != tc.want {
				t.Fatalf("IsAuth(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestErrorIsAuthError covers the IsAuthError method that lets *Error satisfy
// the unexported IsAuthError() interface used by cli.ExitCode and
// auth.translateErr. Same classification as IsAuth but callable directly on
// the concrete type.
func TestErrorIsAuthError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  *Error
		want bool
	}{
		{"401 status", &Error{HTTPStatus: 401}, true},
		{"unauthenticated code", &Error{Code: "unauthenticated"}, true},
		{"other", &Error{HTTPStatus: 400, Code: "invalid_argument"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.IsAuthError(); got != tc.want {
				t.Fatalf("IsAuthError() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestErrorIsAuthErrorNilReceiver guards the nil-receiver branch: a typed-nil
// *Error (e.g. one an interface value lifted from a nil return) must return
// false without panicking. Callers that pluck a *Error out of a wrapped chain
// via errors.As can legitimately land on (*Error)(nil); the method must not
// dereference it.
func TestErrorIsAuthErrorNilReceiver(t *testing.T) {
	t.Parallel()
	var e *Error // typed nil
	if got := e.IsAuthError(); got != false {
		t.Fatalf("(*Error)(nil).IsAuthError() = %v, want false", got)
	}
}

// TestIsAuthTypedNilDoesNotPanic guards the typed-nil *Error case inside an
// error-wrapping chain. A helper that returns an error interface holding a
// (*Error)(nil) — for example, a stub that forgot to return a real error —
// used to panic inside IsAuth because errors.As succeeds, then the method
// dereferenced a nil receiver. IsAuth must now report false cleanly.
func TestIsAuthTypedNilDoesNotPanic(t *testing.T) {
	t.Parallel()
	// typedNilError returns an `error` interface whose dynamic value is a
	// typed nil pointer — the exact shape that surfaces when an `if err != nil`
	// guard in the caller is based on an `error`-typed variable assigned
	// from a function returning `*Error`.
	err := typedNilError()
	// Sanity: the bare interface holding a typed-nil is non-nil by Go's
	// comparison rules — this is the very trap that makes IsAuth
	// vulnerable without the nil guard.
	if err == nil {
		t.Fatalf("test setup invariant violated: typed-nil *Error should lift into a non-nil error interface")
	}
	var got bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("IsAuth panicked on typed-nil *Error: %v", r)
			}
		}()
		got = IsAuth(err)
	}()
	if got {
		t.Fatalf("IsAuth(typed-nil *Error) = true, want false")
	}
}
