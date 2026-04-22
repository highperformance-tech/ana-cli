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

func databricksClientCredentialsArgs() []string {
	return []string{
		"databricks", "client-credentials",
		"--name", "db1",
		"--host", "dbc-xxxx.cloud.databricks.com",
		"--http-path", "/sql/1.0/warehouses/abc123",
		"--catalog", "main",
		"--schema", "default",
		"--client-id", "cid",
		"--client-secret", "csec",
	}
}

func runDatabricksClientCredentials(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateDatabricksClientCredentialsHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 55.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	out, err := runDatabricksClientCredentials(t, f.deps(), databricksClientCredentialsArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 55") || !strings.Contains(s, "connectorType: DATABRICKS") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"DATABRICKS"`, `"name":"db1"`, `"authStrategy":"service_role"`,
		`"host":"dbc-xxxx.cloud.databricks.com"`, `"port":443`,
		`"clientCredentials":`, `"clientId":"cid"`, `"clientSecret":"csec"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	for _, unwanted := range []string{`"pat":`, `"oauthU2m":`, `"token":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateDatabricksClientCredentialsSecretStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"databricks", "client-credentials",
		"--name", "db1",
		"--host", "h", "--http-path", "/p", "--catalog", "c", "--schema", "s",
		"--client-id", "cid",
		"--client-secret-stdin",
	}
	_, err := runDatabricksClientCredentials(t, f.deps(), args, "stdin-secret\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"clientSecret":"stdin-secret"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateDatabricksClientCredentialsSecretStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "client-credentials",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	_, err := runDatabricksClientCredentials(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsSecretStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "client-credentials",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsMissingSecret(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "client-credentials",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid",
	}
	_, err := runDatabricksClientCredentials(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksClientCredentials(t, (&fakeDeps{}).deps(), []string{"databricks", "client-credentials"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "http-path", "catalog", "schema", "client-id"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(databricksClientCredentialsArgs(), "--"+flag, "")
			_, err := runDatabricksClientCredentials(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("flag=%s err=%v", flag, err)
			}
		})
	}
}

func TestCreateDatabricksClientCredentialsJSONBypass(t *testing.T) {
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
	if err := g.Run(ctx, databricksClientCredentialsArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateDatabricksClientCredentialsRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), databricksClientCredentialsArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

func TestCreateDatabricksClientCredentialsBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksClientCredentials(t, (&fakeDeps{}).deps(), []string{"databricks", "client-credentials", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runDatabricksClientCredentials(t, f.deps(), databricksClientCredentialsArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksClientCredentialsRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runDatabricksClientCredentials(t, f.deps(), databricksClientCredentialsArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
