package cli

import "fmt"

// Token wraps a bearer token so any accidental `%s`/`%v`/`%q` on a logger or
// error renders the redacted form (same mask RedactToken produces). Code that
// needs the raw value calls Value() — the single explicit escape hatch.
//
// The underlying kind is string, so the type is JSON-transparent (persists and
// loads as a plain string), comparisons against string literals still work,
// and tests can write `cli.Token("abcdefgh")` with no extra plumbing.
type Token string

// String returns the redacted representation. Triggered by any verb that
// doesn't override it (`%s`, `%v`, default Sprintln, error chains, etc.).
func (t Token) String() string { return RedactToken(string(t)) }

// Format also intercepts `%q`, `%+v`, and `%#v`, which would otherwise bypass
// String() and dump the raw bytes (fmt special-cases string kinds for %q/%#v
// specifically). Returning the same redacted form for every verb means no
// format directive can accidentally leak a token.
func (t Token) Format(f fmt.State, _ rune) { fmt.Fprint(f, t.String()) }

// Value returns the raw token string. This is the intended escape hatch for
// the two legitimate consumers: the transport layer building an Authorization
// header, and the auth-keys verb that prints a freshly minted key once.
// Adding new callers is a design smell — prefer passing the Token through.
func (t Token) Value() string { return string(t) }
