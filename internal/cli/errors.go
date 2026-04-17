package cli

import "errors"

// ErrUsage marks a usage-related failure. Commands that want the root to exit
// with code 1 should return an error that wraps ErrUsage via %w.
var ErrUsage = errors.New("usage")

// authError is an optional interface a transport/auth error can implement so
// the root dispatcher can map it to exit code 3 without importing that package.
type authError interface {
	IsAuthError() bool
}

// ExitCode maps a returned error to the process exit code.
//
//	nil         -> 0
//	ErrUsage    -> 1  (including wrapped)
//	authError   -> 3  (including wrapped; IsAuthError must return true)
//	otherwise   -> 2
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrUsage) {
		return 1
	}
	var ae authError
	if errors.As(err, &ae) && ae.IsAuthError() {
		return 3
	}
	return 2
}
