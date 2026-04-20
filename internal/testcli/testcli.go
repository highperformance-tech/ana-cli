// Package testcli provides helpers for verb-package unit tests. Mirrors the
// stdlib split (httptest, iotest) — production code in internal/cli, test
// scaffolding here, so the cli package itself stays free of test-only types.
package testcli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// FailingWriter is an io.Writer that always returns an error. Verb tests use
// it as a Stdout to exercise write/flush error branches (tabwriter.Flush,
// encoder.Encode) without relying on platform-specific file state.
type FailingWriter struct{}

// Write implements io.Writer and always returns the sentinel error so tests
// can match on the string.
func (FailingWriter) Write([]byte) (int, error) { return 0, errors.New("w boom") }

// FailingIO returns a cli.IO whose Stdout always fails writes. Stdin is an
// empty reader, Stderr is a fresh bytes.Buffer, Env returns "" for every key,
// and Now returns the same fixed epoch as NewIO so tests that assert on
// time-derived output stay deterministic regardless of which constructor they
// picked. Shared across verb tests that need to surface the write-error
// branch — duplicating the literal at each call site was noise.
func FailingIO() cli.IO {
	return cli.IO{
		Stdin:  strings.NewReader(""),
		Stdout: FailingWriter{},
		Stderr: &bytes.Buffer{},
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}
}

// NewIO returns a cli.IO wired to in-memory buffers so verb tests can assert
// on stdout/stderr without touching real file descriptors. Stdin defaults to
// an empty reader when nil — most tests don't exercise stdin. Env returns ""
// for every key, Now returns a fixed epoch so time-dependent assertions stay
// deterministic.
func NewIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}
