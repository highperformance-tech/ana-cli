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

func snowflakeOAuthSSOArgs() []string {
	return []string{
		"snowflake", "oauth-sso",
		"--name", "sf1",
		"--locator", "abc12345.us-east-1",
		"--database", "D",
		"--oauth-client-id", "cid",
		"--oauth-client-secret", "csec",
	}
}

func runSnowflakeOAuthSSO(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateSnowflakeOAuthSSOHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 55.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	out, err := runSnowflakeOAuthSSO(t, f.deps(), snowflakeOAuthSSOArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 55") || !strings.Contains(s, "complete OAuth at https://app.textql.com") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"SNOWFLAKE"`, `"name":"sf1"`, `"authStrategy":"oauth_sso"`,
		`"locator":"abc12345.us-east-1"`, `"database":"D"`,
		`"oauthClientId":"cid"`, `"oauthClientSecret":"csec"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	// OAuth modes send no username / password / privateKey.
	for _, unwanted := range []string{`"username":`, `"password":`, `"privateKey":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateSnowflakeOAuthSSOCustomEndpoint(t *testing.T) {
	t.Parallel()
	// Self-hosted / non-prod operators resolve an endpoint that is not
	// app.textql.com; the success note must echo that URL so users complete
	// the OAuth handshake in the right web app.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 77.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	deps := f.deps()
	deps.Endpoint = func() string { return "https://staging.example.com" }
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := g.Run(context.Background(), snowflakeOAuthSSOArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "complete OAuth at https://staging.example.com") {
		t.Errorf("stdout=%q missing custom endpoint URL", s)
	}
	// And it must NOT leak the hardcoded default into a non-prod profile.
	if strings.Contains(s, "https://app.textql.com") {
		t.Errorf("stdout=%q leaked default endpoint", s)
	}
}

func TestCreateSnowflakeOAuthSSOSecretStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"snowflake", "oauth-sso",
		"--name", "sf1",
		"--locator", "acct",
		"--database", "D",
		"--oauth-client-id", "cid",
		"--oauth-client-secret-stdin",
	}
	_, err := runSnowflakeOAuthSSO(t, f.deps(), args, "stdin-secret\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"oauthClientSecret":"stdin-secret"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateSnowflakeOAuthSSOSecretStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-sso",
		"--name", "sf1",
		"--locator", "acct",
		"--database", "D",
		"--oauth-client-id", "cid",
		"--oauth-client-secret-stdin",
	}
	_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSOSecretStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-sso",
		"--name", "sf1",
		"--locator", "acct",
		"--database", "D",
		"--oauth-client-id", "cid",
		"--oauth-client-secret-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSOMissingSecret(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-sso",
		"--name", "sf1",
		"--locator", "acct",
		"--database", "D",
		"--oauth-client-id", "cid",
	}
	_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSOMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), []string{"snowflake", "oauth-sso"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSOEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "locator", "database", "oauth-client-id"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(snowflakeOAuthSSOArgs(), "--"+flag, "")
			_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateSnowflakeOAuthSSOOptionalFields(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	args := append(snowflakeOAuthSSOArgs(),
		"--warehouse", "W",
		"--schema", "S",
		"--role", "R",
	)
	_, err := runSnowflakeOAuthSSO(t, f.deps(), args, "")
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

func TestCreateSnowflakeOAuthSSOJSONBypass(t *testing.T) {
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
	if err := g.Run(ctx, snowflakeOAuthSSOArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateSnowflakeOAuthSSORenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), snowflakeOAuthSSOArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

// TestCreateSnowflakeOAuthSSORejectsExtraPositionals pins the no-positional
// contract for the deeply-nested leaf: trailing tokens after the verb path
// must yield ErrUsage before RequireFlags or any RPC fires.
func TestCreateSnowflakeOAuthSSORejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), []string{"snowflake", "oauth-sso", "extra"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestCreateSnowflakeOAuthSSOBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeOAuthSSO(t, (&fakeDeps{}).deps(), []string{"snowflake", "oauth-sso", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSOUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runSnowflakeOAuthSSO(t, f.deps(), snowflakeOAuthSSOArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthSSORemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runSnowflakeOAuthSSO(t, f.deps(), snowflakeOAuthSSOArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
