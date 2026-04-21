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

// snowflakePasswordArgs returns the full dispatch args for
// `connector create snowflake password ...` with every required flag set.
// Routes through newCreateGroup so ancestor-flag plumbing (--name,
// --locator, --database, etc. declared on the Snowflake Group) is exercised.
func snowflakePasswordArgs() []string {
	return []string{
		"snowflake", "password",
		"--name", "sf1",
		"--locator", "abc12345.us-east-1",
		"--database", "D",
		"--user", "U",
		"--password", "p",
	}
}

func runSnowflakePassword(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateSnowflakePasswordHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 42.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	out, err := runSnowflakePassword(t, f.deps(), snowflakePasswordArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 42") || !strings.Contains(s, "name: sf1") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"SNOWFLAKE"`, `"name":"sf1"`, `"authStrategy":"service_role"`,
		`"snowflake":`, `"locator":"abc12345.us-east-1"`, `"database":"D"`,
		`"username":"U"`, `"password":"p"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	// Optional fields not set → omitted.
	for _, unwanted := range []string{`"warehouse":`, `"schema":`, `"role":`, `"privateKey":`, `"oauthClientId":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
	if f.lastPath != servicePath+"/CreateConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestCreateSnowflakePasswordOptionalFields(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	args := append(snowflakePasswordArgs(),
		"--warehouse", "W",
		"--schema", "S",
		"--role", "R",
	)
	_, err := runSnowflakePassword(t, f.deps(), args, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{`"warehouse":"W"`, `"schema":"S"`, `"role":"R"`} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
}

func TestCreateSnowflakePasswordJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	g := newCreateGroup(f.deps())
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(ctx, snowflakePasswordArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateSnowflakePasswordStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"snowflake", "password",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
		"--password-stdin",
	}
	_, err := runSnowflakePassword(t, f.deps(), args, "secret-line\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"secret-line"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateSnowflakePasswordStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "password",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
		"--password-stdin",
	}
	_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "password",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
		"--password-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordMissingPassword(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "password",
		"--name", "n",
		"--locator", "acct",
		"--database", "D",
		"--user", "U",
	}
	_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordMissingFlags(t *testing.T) {
	t.Parallel()
	args := []string{"snowflake", "password"}
	_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "locator", "database", "user"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(snowflakePasswordArgs(), "--"+flag, "")
			_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateSnowflakePasswordRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), snowflakePasswordArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

func TestCreateSnowflakePasswordBadFlag(t *testing.T) {
	t.Parallel()
	args := []string{"snowflake", "password", "--nope"}
	_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runSnowflakePassword(t, f.deps(), snowflakePasswordArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakePasswordRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runSnowflakePassword(t, f.deps(), snowflakePasswordArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateGroupUnknownSnowflakeAuthMode(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakePassword(t, (&fakeDeps{}).deps(), []string{"snowflake", "certificate"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}
