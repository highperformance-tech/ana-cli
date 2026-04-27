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

// databricksAccessTokenArgs returns the full dispatch args for
// `connector create databricks access-token ...` with every required flag
// set (token via --token for the happy path; the stdin variants build args
// inline). Routes through newCreateGroup so ancestor-flag plumbing declared
// on the Databricks Group (--name, --host, --http-path, --port, --catalog,
// --schema) is exercised.
func databricksAccessTokenArgs() []string {
	return []string{
		"databricks", "access-token",
		"--name", "db1",
		"--host", "dbc-xxxx.cloud.databricks.com",
		"--http-path", "/sql/1.0/warehouses/abc123",
		"--catalog", "main",
		"--schema", "default",
		"--token", "pat-tok",
	}
}

func runDatabricksAccessToken(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateDatabricksAccessTokenHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 42.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	out, err := runDatabricksAccessToken(t, f.deps(), databricksAccessTokenArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 42") || !strings.Contains(s, "name: db1") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"DATABRICKS"`, `"name":"db1"`, `"authStrategy":"service_role"`,
		`"databricks":`, `"host":"dbc-xxxx.cloud.databricks.com"`,
		`"httpPath":"/sql/1.0/warehouses/abc123"`, `"port":443`,
		`"catalog":"main"`, `"schema":"default"`,
		`"databricksAuth":`, `"pat":`, `"token":"pat-tok"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	// Other oneof variants must be absent.
	for _, unwanted := range []string{`"clientCredentials":`, `"oauthU2m":`, `"snowflake":`, `"postgres":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
	if f.lastPath != servicePath+"/CreateConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestCreateDatabricksAccessTokenCustomPort(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	args := append(databricksAccessTokenArgs(), "--port", "8443")
	_, err := runDatabricksAccessToken(t, f.deps(), args, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"port":8443`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateDatabricksAccessTokenTokenStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"databricks", "access-token",
		"--name", "db1",
		"--host", "h", "--http-path", "/p", "--catalog", "c", "--schema", "s",
		"--token-stdin",
	}
	_, err := runDatabricksAccessToken(t, f.deps(), args, "stdin-tok\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"token":"stdin-tok"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateDatabricksAccessTokenStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "access-token",
		"--name", "n", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--token-stdin",
	}
	_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "access-token",
		"--name", "n", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--token-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenMissingToken(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "access-token",
		"--name", "n", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
	}
	_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), []string{"databricks", "access-token"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "http-path", "catalog", "schema"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(databricksAccessTokenArgs(), "--"+flag, "")
			_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateDatabricksAccessTokenPortRange(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"0", "-1", "65536", "100000"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			args := append(databricksAccessTokenArgs(), "--port", port)
			_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--port") {
				t.Errorf("port=%s err=%v", port, err)
			}
		})
	}
}

func TestCreateDatabricksAccessTokenJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "n", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	g := newCreateGroup(f.deps())
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(ctx, databricksAccessTokenArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateDatabricksAccessTokenRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), databricksAccessTokenArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

// TestCreateDatabricksAccessTokenRejectsExtraPositionals pins the
// no-positional contract for the deeply-nested leaf: trailing tokens after
// the verb path must yield ErrUsage before RequireFlags or any RPC fires.
// Use the happy-path argv plus a trailing positional so the assertion would
// fail loudly if the leaf's positional check ever moved AFTER RequireFlags.
func TestCreateDatabricksAccessTokenRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := append(databricksAccessTokenArgs(), "extra")
	_, err := runDatabricksAccessToken(t, f.deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want positional ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestCreateDatabricksAccessTokenBadFlag(t *testing.T) {
	t.Parallel()
	args := []string{"databricks", "access-token", "--nope"}
	_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runDatabricksAccessToken(t, f.deps(), databricksAccessTokenArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksAccessTokenRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runDatabricksAccessToken(t, f.deps(), databricksAccessTokenArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateGroupUnknownDatabricksAuthMode(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksAccessToken(t, (&fakeDeps{}).deps(), []string{"databricks", "basic-auth"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}
