package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- path dispatch ---

func TestRPCShortFormPrefixed(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"foo.Bar/Baz"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.lastPath != "/rpc/public/foo.Bar/Baz" {
		t.Errorf("path = %q, want /rpc/public/foo.Bar/Baz", f.lastPath)
	}
	if f.lastMethod != "POST" {
		t.Errorf("method = %q, want POST", f.lastMethod)
	}
	// No --data, no GET → default body is `{}`.
	if string(f.lastBody) != "{}" {
		t.Errorf("body = %q, want {}", f.lastBody)
	}
}

func TestRESTLeadingSlashPassthrough(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"/v1/things"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.lastPath != "/v1/things" {
		t.Errorf("path = %q", f.lastPath)
	}
}

func TestRPCLeadingSlashPassthrough(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"/rpc/public/foo/Bar"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.lastPath != "/rpc/public/foo/Bar" {
		t.Errorf("path = %q", f.lastPath)
	}
}

// --- method / body ---

func TestMethodGETHasNoBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--method", "GET", "/v1/things"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.lastMethod != "GET" {
		t.Errorf("method = %q", f.lastMethod)
	}
	if f.lastBody != nil {
		t.Errorf("body = %q, want nil for GET", f.lastBody)
	}
}

func TestMethodHEADHasNoBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--method", "HEAD", "/v1/things"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.lastBody != nil {
		t.Errorf("body = %q, want nil for HEAD", f.lastBody)
	}
}

func TestDataFlagUsedAsBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(),
		[]string{"--data", `{"x":1}`, "foo/Bar"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(f.lastBody) != `{"x":1}` {
		t.Errorf("body = %q", f.lastBody)
	}
}

func TestDataStdinUsedAsBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(strings.NewReader(`{"chatId":"abc"}`))
	err := cmd.Run(context.Background(),
		[]string{"--data-stdin", "foo/Bar"}, stdio)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(f.lastBody) != `{"chatId":"abc"}` {
		t.Errorf("body = %q", f.lastBody)
	}
}

// errReader always fails — exercises the io.ReadAll branch in resolveBody.
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("stdin boom") }

func TestDataStdinReadErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(errReader{})
	err := cmd.Run(context.Background(),
		[]string{"--data-stdin", "foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read stdin") {
		t.Fatalf("want read stdin error, got %v", err)
	}
}

func TestDataAndDataStdinMutuallyExclusive(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(),
		[]string{"--data", "{}", "--data-stdin", "foo/Bar"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err = %v, want ErrUsage", err)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("err = %v, want mutual-exclusion message", err)
	}
}

// --- validation ---

func TestMissingPathIsUsageError(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err = %v, want ErrUsage", err)
	}
}

func TestBlankPathIsUsageError(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	// A whitespace-only arg is just as useless as a missing one.
	err := cmd.Run(context.Background(), []string{"   "}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err = %v, want ErrUsage", err)
	}
}

func TestEmptyMethodIsUsageError(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--method", "", "foo/Bar"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err = %v, want ErrUsage", err)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope", "foo/Bar"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err = %v, want ErrUsage", err)
	}
}

// --- DoRaw error / non-2xx ---

func TestTransportErrorWrapped(t *testing.T) {
	t.Parallel()
	boom := errors.New("dial fail")
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 0, nil, boom
	}}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("want wrapped %v, got %v", boom, err)
	}
	if !strings.HasPrefix(err.Error(), "api:") {
		t.Errorf("err missing api: prefix: %v", err)
	}
}

func TestNon2xxBodyToStderr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 404, []byte(`{"code":"not_found"}`), nil
	}}
	cmd := New(f.deps())
	stdio, out, errb := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty on non-2xx, got %q", out.String())
	}
	if !strings.Contains(errb.String(), "not_found") {
		t.Errorf("stderr missing body: %q", errb.String())
	}
	// Body did not end in newline, so one should be appended so the caller's
	// prompt isn't glued to the response.
	if !strings.HasSuffix(errb.String(), "\n") {
		t.Errorf("stderr should end in newline, got %q", errb.String())
	}
}

func TestNon2xxBodyAlreadyEndsInNewline(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 500, []byte("{}\n"), nil
	}}
	cmd := New(f.deps())
	stdio, _, errb := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil {
		t.Fatal("expected error")
	}
	// One newline total — we shouldn't double it.
	if strings.HasSuffix(errb.String(), "\n\n") {
		t.Errorf("stderr double-newlined: %q", errb.String())
	}
}

func TestNon2xxEmptyBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 400, nil, nil
	}}
	cmd := New(f.deps())
	stdio, _, errb := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("err = %v", err)
	}
	if errb.Len() != 0 {
		t.Errorf("stderr should be empty for empty body, got %q", errb.String())
	}
}

func TestNon2xxStderrWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 500, []byte(`{"code":"x"}`), nil
	}}
	cmd := New(f.deps())
	stdio := testcli.FailingIO()
	// FailingIO.Stdout is the failing writer; swap stderr too via a copy.
	stdio.Stderr = testcli.FailingWriter{}
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Fatalf("err = %v, want stderr write error", err)
	}
}

func TestNon2xxStderrTrailingNewlineWriteErr(t *testing.T) {
	t.Parallel()
	// Body lacks trailing newline → emitError adds one. A writer that accepts
	// the body then refuses the trailing newline exercises the second Fprintln
	// branch.
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 500, []byte(`{"code":"x"}`), nil // no \n
	}}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	stdio.Stderr = &acceptThenFail{}
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "api:") {
		t.Fatalf("err = %v, want api: wrap of trailing-newline write error", err)
	}
}

// --- 2xx happy output ---

func TestSuccessPrettyPrint(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte(`{"a":1,"b":2}`), nil
	}}
	cmd := New(f.deps())
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio); err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	// Pretty output has newlines + 2-space indent between keys.
	if !strings.Contains(s, "\n  \"a\": 1,") {
		t.Errorf("expected indented output, got %q", s)
	}
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("expected trailing newline, got %q", s)
	}
}

func TestSuccessRawPassthrough(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte(`{"a":1}`), nil
	}}
	cmd := New(f.deps())
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"--raw", "foo/Bar"}, stdio); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.String() != `{"a":1}` {
		t.Errorf("raw passthrough expected verbatim bytes, got %q", out.String())
	}
}

func TestSuccessNonJSONFallsThroughToRaw(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte("plain text"), nil
	}}
	cmd := New(f.deps())
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.String() != "plain text" {
		t.Errorf("non-JSON body should pass through verbatim, got %q", out.String())
	}
}

func TestSuccessEmptyBody(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 204, nil, nil
	}}
	cmd := New(f.deps())
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected empty stdout, got %q", out.String())
	}
}

func TestSuccessRawStdoutErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte(`{"a":1}`), nil
	}}
	cmd := New(f.deps())
	stdio := testcli.FailingIO()
	err := cmd.Run(context.Background(), []string{"--raw", "foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Fatalf("err = %v, want stdout write error", err)
	}
}

func TestSuccessPrettyStdoutErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte(`{"a":1}`), nil
	}}
	cmd := New(f.deps())
	stdio := testcli.FailingIO()
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Fatalf("err = %v, want stdout write error", err)
	}
}

func TestSuccessPrettyTrailingNewlineStdoutErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte(`{"a":1}`), nil
	}}
	cmd := New(f.deps())
	stdio, _, _ := testcli.NewIO(nil)
	// Accept the pretty body, then fail on the trailing newline write.
	stdio.Stdout = &acceptThenFail{}
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "api:") {
		t.Fatalf("err = %v, want api: wrap of trailing-newline write error", err)
	}
}

func TestSuccessNonJSONStdoutErr(t *testing.T) {
	t.Parallel()
	// Body isn't JSON → json.Indent fails → fallthrough to raw write. With a
	// failing stdout the raw write errors, exercising the inner branch.
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte("plain text"), nil
	}}
	cmd := New(f.deps())
	stdio := testcli.FailingIO()
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Fatalf("err = %v, want stdout write error on non-JSON fallthrough", err)
	}
}

func TestSuccessRawEmptyBodyStdoutErr(t *testing.T) {
	t.Parallel()
	// Empty body + --raw takes the `c.raw || len(body) == 0` branch but
	// stdio.Stdout.Write on 0 bytes still returns (0, nil) for a FailingWriter?
	// FailingWriter always errors — so this exercises the Write-err branch
	// with an empty body input.
	f := &fakeDeps{doRawFn: func(context.Context, string, string, []byte) (int, []byte, error) {
		return 200, []byte{}, nil
	}}
	cmd := New(f.deps())
	stdio := testcli.FailingIO()
	err := cmd.Run(context.Background(), []string{"foo/Bar"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Fatalf("err = %v, want stdout write error on empty-body raw path", err)
	}
}

// acceptThenFail accepts the first Write call completely, then fails every
// subsequent call. Exercises the "body wrote OK but trailing-newline write
// failed" branches in emitError and emitSuccess without per-test byte counts.
type acceptThenFail struct{ firstDone bool }

func (w *acceptThenFail) Write(p []byte) (int, error) {
	if !w.firstDone {
		w.firstDone = true
		return len(p), nil
	}
	return 0, errors.New("w boom")
}
