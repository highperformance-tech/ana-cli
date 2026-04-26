package connector

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// requiredArgs returns the full dispatch args for `connector create postgres
// password ...` with every required flag set. Tests route through
// newCreateGroup so ancestor flags (--name/--ssl declared on the Postgres
// Group) get bound on the leaf's FlagSet the same way real dispatch does.
func requiredArgs() []string {
	return []string{
		"postgres", "password",
		"--name", "pg1",
		"--host", "h",
		"--port", "5432",
		"--user", "u",
		"--database", "d",
		"--password", "p",
	}
}

// runCreate dispatches the Group end-to-end. Returns the out buffer so
// happy-path tests can assert on stdout.
func runCreate(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreatePostgresPasswordHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	out, err := runCreate(t, f.deps(), requiredArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 99") || !strings.Contains(s, "name: pg1") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"POSTGRES"`, `"name":"pg1"`,
		`"postgres":`, `"host":"h"`, `"port":5432`, `"user":"u"`,
		`"password":"p"`, `"database":"d"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	if f.lastPath != servicePath+"/CreateConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestCreatePostgresPasswordSSL(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	args := append(requiredArgs(), "--ssl")
	_, err := runCreate(t, f.deps(), args, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"sslMode":true`) {
		t.Errorf("--ssl should wire sslMode:true; req=%s", string(f.lastRawReq))
	}
}

func TestCreatePostgresPasswordJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "POSTGRES"}
			return nil
		},
	}
	// Wrap dispatch with ctx carrying JSON=true. Run the Group with that ctx.
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	g := newCreateGroup(f.deps())
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(ctx, requiredArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreatePostgresPasswordStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"postgres", "password",
		"--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	_, err := runCreate(t, f.deps(), args, "secret-line\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"secret-line"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreatePostgresPasswordStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"postgres", "password",
		"--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePostgresPasswordStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"postgres", "password",
		"--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestResolvePasswordNilReader(t *testing.T) {
	t.Parallel()
	_, err := resolveSecret("password", "", true, nil)
	if err == nil {
		t.Errorf("want error on nil reader")
	}
}

// TestResolvePasswordPreservesSurroundingWhitespace locks in the contract
// that stdin passwords are not silently trimmed: a credential can
// legitimately start or end with spaces or tabs, and mutating user-supplied
// bytes would cause hard-to-diagnose auth failures. cli.ReadPassword strips
// only the trailing line terminator (\n or \r\n) and nothing else.
func TestResolvePasswordPreservesSurroundingWhitespace(t *testing.T) {
	t.Parallel()
	got, err := resolveSecret("password", "", true, strings.NewReader(" secret\twith\tabs \n"))
	if err != nil {
		t.Fatalf("resolveSecret: %v", err)
	}
	if want := " secret\twith\tabs "; got != want {
		t.Errorf("resolveSecret=%q want %q", got, want)
	}
}

// TestResolvePasswordStripsCRLF verifies a Windows line terminator is
// stripped cleanly without swallowing any user bytes that precede it.
func TestResolvePasswordStripsCRLF(t *testing.T) {
	t.Parallel()
	got, err := resolveSecret("password", "", true, strings.NewReader(" hunter2 \r\n"))
	if err != nil {
		t.Fatalf("resolveSecret: %v", err)
	}
	if want := " hunter2 "; got != want {
		t.Errorf("resolveSecret=%q want %q", got, want)
	}
}

func TestCreatePostgresPasswordMissingPassword(t *testing.T) {
	t.Parallel()
	args := []string{
		"postgres", "password",
		"--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
	}
	_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePostgresPasswordMissingFlags(t *testing.T) {
	t.Parallel()
	args := []string{"postgres", "password"}
	_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePostgresPasswordEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "user", "database"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(requiredArgs(), "--"+flag, "")
			_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreatePostgresPasswordBadPort(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"0", "-1", "70000"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			args := []string{
				"postgres", "password",
				"--name", "n", "--host", "h",
				"--port", port, "--user", "u", "--database", "d", "--password", "p",
			}
			_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreatePostgresPasswordRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), requiredArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

// TestCreatePostgresPasswordRejectsExtraPositionals pins the no-positional
// contract for the deeply-nested leaf: trailing tokens after the verb path
// must yield ErrUsage before RequireFlags or any RPC fires.
func TestCreatePostgresPasswordRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	_, err := runCreate(t, (&fakeDeps{}).deps(), []string{"postgres", "password", "extra"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestCreatePostgresPasswordBadFlag(t *testing.T) {
	t.Parallel()
	args := []string{"postgres", "password", "--nope"}
	_, err := runCreate(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePostgresPasswordUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runCreate(t, f.deps(), requiredArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePostgresPasswordRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runCreate(t, f.deps(), requiredArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

// TestCreateGroupUnknownDialect exercises the top-level create Group: an
// unknown dialect (and implicitly, an unknown auth-mode under a dialect)
// returns ErrUsage via cli.Group dispatch.
func TestCreateGroupUnknownDialect(t *testing.T) {
	t.Parallel()
	_, err := runCreate(t, (&fakeDeps{}).deps(), []string{"mysql"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestCreateGroupUnknownAuthMode mirrors the above but under a known dialect.
func TestCreateGroupUnknownAuthMode(t *testing.T) {
	t.Parallel()
	_, err := runCreate(t, (&fakeDeps{}).deps(), []string{"postgres", "certificate"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestCreateGroupHelpMentionsDialects — the `create` Group's Help() should
// list every registered dialect child so users discover them via
// `ana connector create --help`.
func TestCreateGroupHelpMentionsDialects(t *testing.T) {
	t.Parallel()
	g := newCreateGroup(Deps{})
	h := g.Help()
	for _, d := range []string{"postgres", "snowflake", "databricks"} {
		if !strings.Contains(h, d) {
			t.Errorf("create Help missing dialect %q: %q", d, h)
		}
	}
}
