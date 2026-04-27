package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestLoginLineMode(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader("my-token\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if f.saved == nil || f.saved.Token != "my-token" {
		t.Errorf("saved=%+v want token=my-token", f.saved)
	}
	if f.saved.Endpoint != DefaultEndpoint {
		t.Errorf("endpoint=%q want default", f.saved.Endpoint)
	}
	if !strings.Contains(out.String(), "saved to /tmp/ana/config.json") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLoginTokenStdinFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	// Multi-line token (JWT style) + trailing newline. --token-stdin should
	// consume the whole stream and trim.
	stdio, _, _ := testcli.NewIO(strings.NewReader("line1\nline2\n  \n"))
	err := New(f.deps()).Run(context.Background(), []string{"login", "--token-stdin"}, stdio)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if f.saved.Token != "line1\nline2" {
		t.Errorf("saved token=%q", f.saved.Token)
	}
}

func TestLoginEndpointPrecedenceGlobal(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{Endpoint: "https://loaded"}, nil }}
	cmd := &loginCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{Endpoint: "https://override"})
	stdio, _, _ := testcli.NewIO(strings.NewReader("tok\n"))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved.Endpoint != "https://override" {
		t.Errorf("endpoint=%q want https://override", f.saved.Endpoint)
	}
}

func TestLoginEndpointPrecedenceLoaded(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{Endpoint: "https://loaded"}, nil }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader("tok\n"))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved.Endpoint != "https://loaded" {
		t.Errorf("endpoint=%q want https://loaded", f.saved.Endpoint)
	}
}

func TestLoginEmptyTokenUsage(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader("\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLoginLoadConfigError(t *testing.T) {
	t.Parallel()
	boom := errors.New("disk boom")
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestLoginSaveConfigError(t *testing.T) {
	t.Parallel()
	boom := errors.New("save boom")
	f := &fakeDeps{saveFn: func(Config) error { return boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestLoginConfigPathError(t *testing.T) {
	t.Parallel()
	boom := errors.New("path boom")
	f := &fakeDeps{pathFn: func() (string, error) { return "", boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(out.String(), "saved") {
		t.Errorf("stdout=%q should still say saved", out.String())
	}
}

// TestLoginRejectsExtraPositionals pins the no-positional contract: any
// trailing token after the verb path must yield ErrUsage before stdin is
// consulted.
func TestLoginRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("stdin must not be read")})
	err := New(f.deps()).Run(context.Background(), []string{"login", "unexpected"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.saved != nil {
		t.Errorf("Save should not be called on positional-arity failure: saved=%+v", f.saved)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestLoginBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New(f.deps()).Run(context.Background(), []string{"login", "--no-such"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if f.saved != nil {
		t.Errorf("Save should not be called on bad-flag failure: saved=%+v", f.saved)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on bad-flag failure: path=%q", f.lastPath)
	}
}

// errReader returns err on first Read so readToken exercises its error paths.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestLoginStdinReadError_TokenStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := New(f.deps()).Run(context.Background(), []string{"login", "--token-stdin"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestLoginStdinReadError_LineMode(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}
