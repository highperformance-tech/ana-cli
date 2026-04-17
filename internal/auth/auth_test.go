package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// --- fakes and helpers ---

// fakeDeps is the table-driven fake for Deps. Each function field defaults to
// a benign implementation; individual tests override what they need. Every
// call is recorded so assertions can inspect the request payload that flowed
// through Unary.
type fakeDeps struct {
	unaryFn    func(ctx context.Context, path string, req, resp any) error
	loadFn     func() (Config, error)
	saveFn     func(Config) error
	pathFn     func() (string, error)
	lastPath   string
	lastReq    any
	lastRawReq []byte
	saved      *Config
	saveCalls  int
	loadCalls  int
	pathCalls  int
}

// deps returns a Deps whose functions funnel through the fake so tests can
// assert on recorded inputs after the command runs.
func (f *fakeDeps) deps() Deps {
	return Deps{
		Unary: func(ctx context.Context, path string, req, resp any) error {
			f.lastPath = path
			f.lastReq = req
			// Capture the JSON-encoded request so tests can assert on exact
			// wire-level field names (camelCase check).
			if b, err := json.Marshal(req); err == nil {
				f.lastRawReq = b
			}
			if f.unaryFn != nil {
				return f.unaryFn(ctx, path, req, resp)
			}
			return nil
		},
		LoadCfg: func() (Config, error) {
			f.loadCalls++
			if f.loadFn != nil {
				return f.loadFn()
			}
			return Config{}, nil
		},
		SaveCfg: func(c Config) error {
			f.saveCalls++
			f.saved = &c
			if f.saveFn != nil {
				return f.saveFn(c)
			}
			return nil
		},
		ConfigPath: func() (string, error) {
			f.pathCalls++
			if f.pathFn != nil {
				return f.pathFn()
			}
			return "/tmp/ana/config.json", nil
		},
	}
}

// newIO builds a cli.IO with in-memory streams and an explicit stdin reader.
func newIO(stdin io.Reader) (cli.IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return cli.IO{
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
		Env:    func(string) string { return "" },
		Now:    func() time.Time { return time.Unix(0, 0) },
	}, &out, &errb
}

// --- New / Group surface ---

func TestNewReturnsGroupWithExpectedChildren(t *testing.T) {
	f := &fakeDeps{}
	g := New(f.deps())
	if g == nil || g.Children == nil {
		t.Fatalf("New returned empty group")
	}
	expected := []string{"login", "logout", "whoami", "keys", "service-accounts"}
	for _, name := range expected {
		if _, ok := g.Children[name]; !ok {
			t.Errorf("missing child %q", name)
		}
	}
	if g.Summary == "" {
		t.Errorf("Summary should be non-empty")
	}
	// keys and service-accounts must themselves be groups with children.
	if kg, ok := g.Children["keys"].(*cli.Group); !ok || len(kg.Children) != 4 {
		t.Errorf("keys should be a group with 4 children")
	}
	if sg, ok := g.Children["service-accounts"].(*cli.Group); !ok || len(sg.Children) != 3 {
		t.Errorf("service-accounts should be a group with 3 children")
	}
}

// --- Help() text coverage ---

func TestHelpStringsNonEmpty(t *testing.T) {
	f := &fakeDeps{}
	cases := map[string]cli.Command{
		"login":   &loginCmd{deps: f.deps()},
		"logout":  &logoutCmd{deps: f.deps()},
		"whoami":  &whoamiCmd{deps: f.deps()},
		"list":    &keysListCmd{deps: f.deps()},
		"create":  &keysCreateCmd{deps: f.deps()},
		"rotate":  &keysRotateCmd{deps: f.deps()},
		"revoke":  &keysRevokeCmd{deps: f.deps()},
		"saList":  &saListCmd{deps: f.deps()},
		"saCreat": &saCreateCmd{deps: f.deps()},
		"saDel":   &saDeleteCmd{deps: f.deps()},
	}
	for n, c := range cases {
		h := c.Help()
		if h == "" {
			t.Errorf("%s: empty help", n)
		}
		if !strings.Contains(strings.ToLower(h), "usage") {
			t.Errorf("%s: help missing usage: %q", n, h)
		}
	}
}

// --- login ---

func TestLoginLineMode(t *testing.T) {
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader("my-token\n"))
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
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	// Multi-line token (JWT style) + trailing newline. --token-stdin should
	// consume the whole stream and trim.
	stdio, _, _ := newIO(strings.NewReader("line1\nline2\n  \n"))
	err := cmd.Run(context.Background(), []string{"--token-stdin"}, stdio)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if f.saved.Token != "line1\nline2" {
		t.Errorf("saved token=%q", f.saved.Token)
	}
}

func TestLoginEndpointPrecedenceGlobal(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{Endpoint: "https://loaded"}, nil }}
	cmd := &loginCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{Endpoint: "https://override"})
	stdio, _, _ := newIO(strings.NewReader("tok\n"))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved.Endpoint != "https://override" {
		t.Errorf("endpoint=%q want https://override", f.saved.Endpoint)
	}
}

func TestLoginEndpointPrecedenceLoaded(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{Endpoint: "https://loaded"}, nil }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader("tok\n"))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved.Endpoint != "https://loaded" {
		t.Errorf("endpoint=%q want https://loaded", f.saved.Endpoint)
	}
}

func TestLoginEmptyTokenUsage(t *testing.T) {
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader("\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLoginLoadConfigError(t *testing.T) {
	boom := errors.New("disk boom")
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestLoginSaveConfigError(t *testing.T) {
	boom := errors.New("save boom")
	f := &fakeDeps{saveFn: func(Config) error { return boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
}

func TestLoginConfigPathError(t *testing.T) {
	boom := errors.New("path boom")
	f := &fakeDeps{pathFn: func() (string, error) { return "", boom }}
	cmd := &loginCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader("tok\n"))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(out.String(), "saved") {
		t.Errorf("stdout=%q should still say saved", out.String())
	}
}

func TestLoginBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--no-such"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// errReader returns err on first Read so readToken exercises its error paths.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestLoginStdinReadError_TokenStdin(t *testing.T) {
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), []string{"--token-stdin"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestLoginStdinReadError_LineMode(t *testing.T) {
	f := &fakeDeps{}
	cmd := &loginCmd{deps: f.deps()}
	stdio, _, _ := newIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestLoginNilStdin(t *testing.T) {
	// readToken rejects nil reader explicitly.
	if _, err := readToken(nil, false); err == nil {
		t.Errorf("want error on nil reader")
	}
}

func TestReadTokenEmptyEOF(t *testing.T) {
	// Fully empty stream: Scanner.Scan() returns false with Err()==nil, so
	// readToken takes the terminal return path.
	tok, err := readToken(strings.NewReader(""), false)
	if err != nil {
		t.Errorf("err=%v", err)
	}
	if tok != "" {
		t.Errorf("tok=%q", tok)
	}
}

// --- logout ---

func TestLogoutClearsToken(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) {
		return Config{Endpoint: "https://x", Token: "secret"}, nil
	}}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.saved == nil || f.saved.Token != "" {
		t.Errorf("saved=%+v want empty token", f.saved)
	}
	if f.saved.Endpoint != "https://x" {
		t.Errorf("endpoint lost: %+v", f.saved)
	}
	if !strings.Contains(out.String(), "logged out") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestLogoutLoadErr(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, errors.New("load boom") }}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Errorf("err=%v", err)
	}
}

func TestLogoutSaveErr(t *testing.T) {
	f := &fakeDeps{saveFn: func(Config) error { return errors.New("save boom") }}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "save boom") {
		t.Errorf("err=%v", err)
	}
}

func TestLogoutUnexpectedArgs(t *testing.T) {
	f := &fakeDeps{}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestLogoutBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &logoutCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

// --- whoami ---

func TestWhoamiHappy(t *testing.T) {
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(ctx context.Context, path string, req, resp any) error {
			if path != "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember" {
				return fmt.Errorf("bad path %s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"member": map[string]any{"emailAddress": "a@b.c"}}
			return nil
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "a@b.c" {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestWhoamiJSON(t *testing.T) {
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(ctx context.Context, path string, req, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"member": map[string]any{"emailAddress": "x@y"}}
			return nil
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"emailAddress\"") {
		t.Errorf("stdout=%q want JSON", out.String())
	}
}

func TestWhoamiNoToken(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, nil }}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("err=%v want ErrNotLoggedIn", err)
	}
}

func TestWhoamiLoadErr(t *testing.T) {
	f := &fakeDeps{loadFn: func() (Config, error) { return Config{}, errors.New("load boom") }}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "load boom") {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiUnaryErr(t *testing.T) {
	f := &fakeDeps{
		loadFn:  func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("network boom") },
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "network boom") {
		t.Errorf("err=%v", err)
	}
}

// stubAuthErr implements authSignaler. Used to verify auth-error translation
// flows up through whoami (but any command's Unary would behave identically).
type stubAuthErr struct{}

func (stubAuthErr) Error() string       { return "remote says no" }
func (stubAuthErr) IsAuthError() bool   { return true }

func TestWhoamiAuthErrTranslated(t *testing.T) {
	f := &fakeDeps{
		loadFn:  func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return stubAuthErr{} },
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	// cli.ExitCode uses errors.As with an IsAuthError()-bearing interface.
	var signaler interface{ IsAuthError() bool }
	if !errors.As(err, &signaler) || !signaler.IsAuthError() {
		t.Errorf("expected translated auth error, got %v", err)
	}
}

func TestWhoamiAuthErrViaString(t *testing.T) {
	// Server returned a plain error with "unauthenticated" in the message —
	// translateErr should still flag it.
	f := &fakeDeps{
		loadFn:  func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("Unauthenticated request") },
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	var signaler interface{ IsAuthError() bool }
	if !errors.As(err, &signaler) || !signaler.IsAuthError() {
		t.Errorf("expected translated auth error, got %v", err)
	}
}

func TestWhoamiMissingEmail(t *testing.T) {
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"member": map[string]any{}}
			return nil
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "emailAddress") {
		t.Errorf("err=%v", err)
	}
}

func TestWhoamiBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &whoamiCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// failingWriter returns err on every Write so we can trip json.Encoder paths.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("w boom") }

func TestWhoamiJSONEncodeError(t *testing.T) {
	f := &fakeDeps{
		loadFn: func() (Config, error) { return Config{Token: "t"}, nil },
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"member": map[string]any{"emailAddress": "x"}}
			return nil
		},
	}
	cmd := &whoamiCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := cli.IO{Stdin: strings.NewReader(""), Stdout: failingWriter{}, Stderr: &bytes.Buffer{}, Env: func(string) string { return "" }, Now: time.Now}
	err := cmd.Run(ctx, nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}

// --- keys list ---

func TestKeysListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"apiKeys": []any{
					map[string]any{"id": "k1", "name": "first", "lastUsedAt": "2026-04-01"},
					map[string]any{"id": "k2", "name": "second"},
				},
			}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "LAST USED") {
		t.Errorf("missing headers: %q", s)
	}
	if !strings.Contains(s, "k1") || !strings.Contains(s, "k2") || !strings.Contains(s, "first") {
		t.Errorf("missing rows: %q", s)
	}
	// Blank lastUsedAt should render as "-".
	if !strings.Contains(s, "-") {
		t.Errorf("expected '-' for empty LAST USED: %q", s)
	}
}

func TestKeysListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"apiKeys": []any{}}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"apiKeys\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestKeysListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// badRaw is a type that round-trips via json fine but unmarshals into our
// typed shape as an error. We use a value that yields invalid JSON for the
// remarshal path by making apiKeys a non-array so Unmarshal into
// listApiKeysResp.APIKeys fails.
func TestKeysListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"apiKeys": "not-an-array"}
			return nil
		},
	}
	cmd := &keysListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// --- keys create ---

func TestKeysCreateHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKey.ID = "k1"
			out.APIKey.Name = "n"
			out.APIKeyHash = "plaintext-token"
			return nil
		},
	}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, out, errb := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "--service-account", "sa-1"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "plaintext-token") {
		t.Errorf("stdout=%q", out.String())
	}
	// The plaintext must be printed to stdout exactly once, with nothing
	// before it (i.e. first line).
	if lines := strings.Count(strings.TrimSpace(out.String()), "\n"); lines != 0 {
		t.Errorf("stdout should have exactly one line, got: %q", out.String())
	}
	if !strings.Contains(errb.String(), "# store this token") {
		t.Errorf("stderr missing note: %q", errb.String())
	}
	// The wire-level request must include camelCase serviceAccountId.
	if !strings.Contains(string(f.lastRawReq), `"serviceAccountId":"sa-1"`) {
		t.Errorf("request=%s", string(f.lastRawReq))
	}
	if !strings.Contains(string(f.lastRawReq), `"name":"n"`) {
		t.Errorf("request=%s", string(f.lastRawReq))
	}
}

func TestKeysCreateOmitsEmptyServiceAccount(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKeyHash = "tok"
			return nil
		},
	}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--name", "n"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRawReq), "serviceAccountId") {
		t.Errorf("serviceAccountId should be omitted: %s", string(f.lastRawReq))
	}
}

func TestKeysCreateMissingName(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestKeysCreateUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysCreateBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- keys rotate ---

func TestKeysRotateHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createApiKeyResp)
			out.APIKeyHash = "new-plaintext"
			return nil
		},
	}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, out, errb := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"k-id"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "new-plaintext") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(errb.String(), "# store this token") {
		t.Errorf("stderr=%q", errb.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"apiKeyId":"k-id"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestKeysRotateMissingPositional(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRotateUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRotateBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysRotateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- keys revoke ---

func TestKeysRevokeHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"k-id"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "revoked k-id") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"apiKeyId":"k-id"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestKeysRevokeMissingPositional(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRevokeUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestKeysRevokeBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &keysRevokeCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- service-accounts list ---

func TestSAListTable(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"serviceAccounts": []any{
					map[string]any{"memberId": "m1", "displayName": "Name", "description": "D"},
					map[string]any{"memberId": "m2", "displayName": "Other", "email": "e@x"},
				},
			}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "NAME") || !strings.Contains(s, "DESCRIPTION") {
		t.Errorf("headers: %q", s)
	}
	// Description fall-through to email when blank.
	if !strings.Contains(s, "e@x") {
		t.Errorf("fallback email missing: %q", s)
	}
}

func TestSAListJSON(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"serviceAccounts": []any{}}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"serviceAccounts\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSAListUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &saListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSAListRemarshalErr(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"serviceAccounts": "nope"}
			return nil
		},
	}
	cmd := &saListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestSAListBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saListCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- service-accounts create ---

func TestSACreateHappy(t *testing.T) {
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createServiceAccountResp)
			out.MemberID = "m1"
			out.Name = "Name"
			return nil
		},
	}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "probe", "--description", "d"}, stdio)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "m1") {
		t.Errorf("stdout=%q", out.String())
	}
	if !strings.Contains(string(f.lastRawReq), `"name":"probe"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
	if !strings.Contains(string(f.lastRawReq), `"description":"d"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSACreateNoRespName(t *testing.T) {
	// Response leaves Name empty; we should echo the request-provided name.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*createServiceAccountResp)
			out.MemberID = "m1"
			return nil
		},
	}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--name", "probe"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "probe") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestSACreateMissingName(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestSACreateUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSACreateBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saCreateCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- service-accounts delete ---

func TestSADeleteHappy(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, out, _ := newIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"m1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted m1") {
		t.Errorf("stdout=%q", out.String())
	}
	// memberId (not serviceAccountId) per catalog.
	if !strings.Contains(string(f.lastRawReq), `"memberId":"m1"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestSADeleteMissingPositional(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestSADeleteUnaryErr(t *testing.T) {
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"id"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestSADeleteBadFlag(t *testing.T) {
	f := &fakeDeps{}
	cmd := &saDeleteCmd{deps: f.deps()}
	stdio, _, _ := newIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// --- translateErr + writeJSON + remarshal direct tests ---

func TestTranslateErrNil(t *testing.T) {
	if translateErr(nil) != nil {
		t.Errorf("nil should pass through")
	}
}

func TestTranslateErrPlainPassthrough(t *testing.T) {
	in := errors.New("random")
	if got := translateErr(in); got != in {
		t.Errorf("plain error mutated: %v", got)
	}
}

func TestAuthErrUnwrapAndError(t *testing.T) {
	inner := errors.New("deep cause")
	wrapped := &authErr{wrapped: inner}
	if wrapped.Error() != "deep cause" {
		t.Errorf("Error()=%q", wrapped.Error())
	}
	if !errors.Is(wrapped, inner) {
		t.Errorf("errors.Is should find inner")
	}
	if !wrapped.IsAuthError() {
		t.Errorf("IsAuthError should return true")
	}
}

func TestWriteJSONErr(t *testing.T) {
	// Pass a value that json.Marshal cannot handle (channel).
	err := writeJSON(&bytes.Buffer{}, make(chan int))
	if err == nil {
		t.Errorf("want error")
	}
}

func TestRemarshalMarshalErr(t *testing.T) {
	// A channel cannot be marshaled.
	if err := remarshal(make(chan int), &struct{}{}); err == nil {
		t.Errorf("want error on unsupported source")
	}
}
