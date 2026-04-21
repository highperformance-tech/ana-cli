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

func snowflakeOAuthIndividualArgs() []string {
	return []string{
		"snowflake", "oauth-individual",
		"--name", "sf1",
		"--locator", "abc12345.us-east-1",
		"--database", "D",
		"--oauth-client-id", "cid",
		"--oauth-client-secret", "csec",
	}
}

func runSnowflakeOAuthIndividual(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateSnowflakeOAuthIndividualHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 66.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	out, err := runSnowflakeOAuthIndividual(t, f.deps(), snowflakeOAuthIndividualArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 66") || !strings.Contains(s, "lazily at first query") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"SNOWFLAKE"`, `"authStrategy":"per_member_oauth"`,
		`"locator":"abc12345.us-east-1"`, `"oauthClientId":"cid"`, `"oauthClientSecret":"csec"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	for _, unwanted := range []string{`"username":`, `"password":`, `"privateKey":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateSnowflakeOAuthIndividualSecretStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"snowflake", "oauth-individual",
		"--name", "sf1", "--locator", "acct", "--database", "D",
		"--oauth-client-id", "cid", "--oauth-client-secret-stdin",
	}
	_, err := runSnowflakeOAuthIndividual(t, f.deps(), args, "stdin-secret\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"oauthClientSecret":"stdin-secret"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateSnowflakeOAuthIndividualSecretStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-individual",
		"--name", "sf1", "--locator", "acct", "--database", "D",
		"--oauth-client-id", "cid", "--oauth-client-secret-stdin",
	}
	_, err := runSnowflakeOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualSecretStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-individual",
		"--name", "sf1", "--locator", "acct", "--database", "D",
		"--oauth-client-id", "cid", "--oauth-client-secret-stdin",
	}
	g := newCreateGroup((&fakeDeps{}).deps())
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := g.Run(context.Background(), args, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualMissingSecret(t *testing.T) {
	t.Parallel()
	args := []string{
		"snowflake", "oauth-individual",
		"--name", "sf1", "--locator", "acct", "--database", "D",
		"--oauth-client-id", "cid",
	}
	_, err := runSnowflakeOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeOAuthIndividual(t, (&fakeDeps{}).deps(), []string{"snowflake", "oauth-individual"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "locator", "database", "oauth-client-id"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(snowflakeOAuthIndividualArgs(), "--"+flag, "")
			_, err := runSnowflakeOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("err=%v", err)
			}
		})
	}
}

func TestCreateSnowflakeOAuthIndividualOptionalFields(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	args := append(snowflakeOAuthIndividualArgs(),
		"--warehouse", "W", "--schema", "S", "--role", "R",
	)
	_, err := runSnowflakeOAuthIndividual(t, f.deps(), args, "")
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

func TestCreateSnowflakeOAuthIndividualJSONBypass(t *testing.T) {
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
	if err := g.Run(ctx, snowflakeOAuthIndividualArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateSnowflakeOAuthIndividualRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "sf1", "connectorType": "SNOWFLAKE"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), snowflakeOAuthIndividualArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

func TestCreateSnowflakeOAuthIndividualBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runSnowflakeOAuthIndividual(t, (&fakeDeps{}).deps(), []string{"snowflake", "oauth-individual", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runSnowflakeOAuthIndividual(t, f.deps(), snowflakeOAuthIndividualArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateSnowflakeOAuthIndividualRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runSnowflakeOAuthIndividual(t, f.deps(), snowflakeOAuthIndividualArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
