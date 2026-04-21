package cli

import "errors"

// ErrUsage marks a usage-related failure — missing arg, unknown command,
// malformed flag. Commands that want the root to exit with code 1 should
// return an error that wraps ErrUsage via %w.
var ErrUsage = errors.New("usage")

// ErrHelp marks an explicit help request (`--help`, `-h`, or `help`). Dispatch
// returns this after printing help text. ExitCode maps it to 0 because the
// user asked for help and got it — that is success, not a usage error.
var ErrHelp = errors.New("help")

// ErrReported marks errors whose diagnostic text has already been written
// to stderr by the callee. main()'s fallback stderr print skips these to
// avoid double-reporting. Wrap with errors.Join(err, ErrReported) after
// emitting the diagnostic yourself.
var ErrReported = errors.New("reported")

// authError is an optional interface a transport/auth error can implement so
// the root dispatcher can map it to exit code 3 without importing that package.
type authError interface {
	IsAuthError() bool
}

// ExitCode maps a returned error to the process exit code.
//
//	nil         -> 0
//	ErrHelp     -> 0  (user asked for help; help was shown)
//	ErrUsage    -> 1  (including wrapped)
//	authError   -> 3  (including wrapped; IsAuthError must return true)
//	otherwise   -> 2
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, ErrHelp) {
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
