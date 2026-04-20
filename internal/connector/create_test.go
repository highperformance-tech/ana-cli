package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// requiredFlags is the minimal happy-path flag set for create.
func requiredFlags() []string {
	return []string{
		"--type", "postgres",
		"--name", "pg1",
		"--host", "h",
		"--port", "5432",
		"--user", "u",
		"--database", "d",
		"--password", "p",
	}
}

func TestCreateHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), requiredFlags(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 99") || !strings.Contains(s, "name: pg1") {
		t.Errorf("stdout=%q", s)
	}
	// Verify camelCase wire fields.
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

func TestCreateJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "POSTGRES"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, requiredFlags(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	// JSON dump path — ensure table-style "connectorId:" formatting isn't used;
	// raw JSON would have `"connectorId":` (with quotes).
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreatePasswordStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &createCmd{deps: f.deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := testcli.NewIO(strings.NewReader("secret-line\n"))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"secret-line"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreatePasswordStdinEmpty(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePasswordStdinReadErr(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
		"--password-stdin",
	}
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreatePasswordStdinNilReader(t *testing.T) {
	t.Parallel()
	// resolvePassword directly, to exercise the nil-reader branch.
	_, err := resolvePassword("", true, nil)
	if err == nil {
		t.Errorf("want error on nil reader")
	}
}

// TestResolvePassword_PreservesSurroundingWhitespace locks in the contract
// that stdin passwords are not silently trimmed: a real credential can
// legitimately start or end with spaces or tabs, and mutating the user-supplied
// bytes would cause hard-to-diagnose auth failures. resolvePassword calls
// cli.ReadPassword, which strips only the trailing line terminator (\n or
// \r\n) and nothing else. This supersedes the prior "TrimsSurroundingWhitespace"
// pin (commit 21a1af9) which had locked in unsafe behavior.
func TestResolvePassword_PreservesSurroundingWhitespace(t *testing.T) {
	t.Parallel()
	got, err := resolvePassword("", true, strings.NewReader(" secret\twith\tabs \n"))
	if err != nil {
		t.Fatalf("resolvePassword: %v", err)
	}
	if want := " secret\twith\tabs "; got != want {
		t.Errorf("resolvePassword=%q want %q", got, want)
	}
}

// TestResolvePassword_StripsCRLF verifies a Windows line terminator is
// stripped cleanly without swallowing any user bytes that precede it.
func TestResolvePassword_StripsCRLF(t *testing.T) {
	t.Parallel()
	got, err := resolvePassword("", true, strings.NewReader(" hunter2 \r\n"))
	if err != nil {
		t.Fatalf("resolvePassword: %v", err)
	}
	if want := " hunter2 "; got != want {
		t.Errorf("resolvePassword=%q want %q", got, want)
	}
}

func TestCreateMissingPassword(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	// Missing both --password and --password-stdin.
	args := []string{
		"--type", "postgres", "--name", "n", "--host", "h",
		"--port", "5432", "--user", "u", "--database", "d",
	}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateWrongType(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	args := []string{"--type", "mysql", "--name", "n"}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateMissingFlags(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	// type is ok but everything else missing.
	args := []string{"--type", "postgres"}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), args, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "user", "database"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			cmd := &createCmd{deps: (&fakeDeps{}).deps()}
			args := append(requiredFlags(), "--"+flag, "")
			stdio, _, _ := testcli.NewIO(strings.NewReader(""))
			err := cmd.Run(context.Background(), args, stdio)
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateBadPort(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"0", "-1", "70000"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			cmd := &createCmd{deps: (&fakeDeps{}).deps()}
			args := []string{
				"--type", "postgres", "--name", "n", "--host", "h",
				"--port", port, "--user", "u", "--database", "d", "--password", "p",
			}
			stdio, _, _ := testcli.NewIO(strings.NewReader(""))
			err := cmd.Run(context.Background(), args, stdio)
			if !errors.Is(err, cli.ErrUsage) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "pg1", "connectorType": "POSTGRES"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	err := cmd.Run(context.Background(), requiredFlags(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

func TestCreateBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &createCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &createCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), requiredFlags(), stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	cmd := &createCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), requiredFlags(), stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
