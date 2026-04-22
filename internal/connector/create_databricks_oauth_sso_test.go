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

func databricksOAuthSSOArgs() []string {
	return []string{
		"databricks", "oauth-sso",
		"--name", "db1",
		"--host", "dbc-xxxx.cloud.databricks.com",
		"--http-path", "/sql/1.0/warehouses/abc123",
		"--catalog", "main",
		"--schema", "default",
		"--client-id", "cid",
		"--client-secret", "csec",
	}
}

func runDatabricksOAuthSSO(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateDatabricksOAuthSSOHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 66.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	out, err := runDatabricksOAuthSSO(t, f.deps(), databricksOAuthSSOArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 66") || !strings.Contains(s, "complete OAuth at https://app.textql.com") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"DATABRICKS"`, `"authStrategy":"oauth_sso"`,
		`"oauthU2m":`, `"clientId":"cid"`, `"clientSecret":"csec"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	// Wire label check: must NOT be `oauthSso` (server rejects that).
	if strings.Contains(req, `"oauthSso":`) {
		t.Errorf("req contains forbidden wire label oauthSso: %s", req)
	}
	// Other databricksAuth variants must be absent.
	for _, unwanted := range []string{`"pat":`, `"clientCredentials":`, `"token":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateDatabricksOAuthSSOCustomEndpoint(t *testing.T) {
	t.Parallel()
	// Self-hosted / non-prod operators resolve an endpoint that is not
	// app.textql.com; the success note must echo that URL so users complete
	// the OAuth handshake in the right web app.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 77.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	deps := f.deps()
	deps.Endpoint = "https://staging.example.com"
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(context.Background(), databricksOAuthSSOArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "complete OAuth at https://staging.example.com") {
		t.Errorf("stdout=%q missing custom endpoint URL", s)
	}
	if strings.Contains(s, "https://app.textql.com") {
		t.Errorf("stdout=%q leaked default endpoint", s)
	}
}

func TestCreateDatabricksOAuthSSOSecretStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"databricks", "oauth-sso",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	_, err := runDatabricksOAuthSSO(t, f.deps(), args, "stdin-secret\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"clientSecret":"stdin-secret"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateDatabricksOAuthSSOSecretStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-sso",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	_, err := runDatabricksOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthSSOSecretStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-sso",
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

func TestCreateDatabricksOAuthSSOMissingSecret(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-sso",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid",
	}
	_, err := runDatabricksOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthSSOMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksOAuthSSO(t, (&fakeDeps{}).deps(), []string{"databricks", "oauth-sso"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthSSOEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "http-path", "catalog", "schema", "client-id"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(databricksOAuthSSOArgs(), "--"+flag, "")
			_, err := runDatabricksOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("flag=%s err=%v", flag, err)
			}
		})
	}
}

func TestCreateDatabricksOAuthSSOJSONBypass(t *testing.T) {
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
	if err := g.Run(ctx, databricksOAuthSSOArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateDatabricksOAuthSSORenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), databricksOAuthSSOArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

func TestCreateDatabricksOAuthSSOBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksOAuthSSO(t, (&fakeDeps{}).deps(), []string{"databricks", "oauth-sso", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthSSOUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runDatabricksOAuthSSO(t, f.deps(), databricksOAuthSSOArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthSSORemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runDatabricksOAuthSSO(t, f.deps(), databricksOAuthSSOArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
