package testcli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestFailingWriterAlwaysErrors(t *testing.T) {
	t.Parallel()
	n, err := FailingWriter{}.Write([]byte("hi"))
	if n != 0 || err == nil {
		t.Fatalf("want (0, err), got (%d, %v)", n, err)
	}
	if !strings.Contains(err.Error(), "w boom") {
		t.Errorf("error text = %q", err.Error())
	}
}

func TestFailingIOShape(t *testing.T) {
	t.Parallel()
	io := FailingIO()
	if io.Stdin == nil || io.Stdout == nil || io.Stderr == nil || io.Env == nil || io.Now == nil {
		t.Fatalf("FailingIO returned zero fields: %+v", io)
	}
	if _, err := io.Stdout.Write([]byte("x")); err == nil {
		t.Errorf("Stdout should fail writes")
	}
	if _, ok := io.Stderr.(*bytes.Buffer); !ok {
		t.Errorf("Stderr should be *bytes.Buffer")
	}
	if got := io.Env("ANYTHING"); got != "" {
		t.Errorf("Env should return empty string, got %q", got)
	}
	// Now must be the same deterministic fixed epoch NewIO uses so tests
	// cannot diverge based on which constructor they picked.
	first := io.Now()
	second := io.Now()
	if !first.Equal(second) {
		t.Errorf("Now should be stable: %v vs %v", first, second)
	}
	if first.IsZero() {
		t.Errorf("Now should return a non-zero time, got %v", first)
	}
}

func TestNewIONilStdinDefaultsToEmptyReader(t *testing.T) {
	t.Parallel()
	ioc, out, errb := NewIO(nil)
	if ioc.Stdin == nil {
		t.Fatalf("Stdin should be non-nil when nil is passed")
	}
	b, err := io.ReadAll(ioc.Stdin)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("expected empty stdin, got %q", b)
	}
	if out == nil || errb == nil {
		t.Errorf("NewIO should return non-nil buffers")
	}
}

func TestNewIOPassesStdinThrough(t *testing.T) {
	t.Parallel()
	ioc, _, _ := NewIO(strings.NewReader("hello"))
	b, err := io.ReadAll(ioc.Stdin)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(b) != "hello" {
		t.Errorf("stdin readback = %q, want %q", b, "hello")
	}
}

func TestNewIOStdoutStderrAccumulate(t *testing.T) {
	t.Parallel()
	ioc, out, errb := NewIO(nil)
	if _, err := ioc.Stdout.Write([]byte("stdout-data")); err != nil {
		t.Fatalf("Stdout.Write returned error: %v", err)
	}
	if _, err := ioc.Stderr.Write([]byte("stderr-data")); err != nil {
		t.Fatalf("Stderr.Write returned error: %v", err)
	}
	if got := out.String(); got != "stdout-data" {
		t.Errorf("out buffer = %q, want %q", got, "stdout-data")
	}
	if got := errb.String(); got != "stderr-data" {
		t.Errorf("errb buffer = %q, want %q", got, "stderr-data")
	}
}

func TestNewIOEnvReturnsEmpty(t *testing.T) {
	t.Parallel()
	ioc, _, _ := NewIO(nil)
	if got := ioc.Env("ANYTHING"); got != "" {
		t.Errorf("Env(ANYTHING) = %q, want empty string", got)
	}
	if got := ioc.Env(""); got != "" {
		t.Errorf("Env(\"\") = %q, want empty string", got)
	}
}

func TestNewIONowIsStableFixedEpoch(t *testing.T) {
	t.Parallel()
	ioc, _, _ := NewIO(nil)
	if ioc.Now == nil {
		t.Fatalf("Now should be non-nil")
	}
	first := ioc.Now()
	second := ioc.Now()
	if !first.Equal(second) {
		t.Errorf("Now should be stable: %v vs %v", first, second)
	}
	// Fixed epoch — Unix(0,0) — is technically non-zero in Go's time.Time
	// sense (IsZero is reserved for the zero value), so assert that.
	if first.IsZero() {
		t.Errorf("Now should return a non-zero time, got %v", first)
	}
}
