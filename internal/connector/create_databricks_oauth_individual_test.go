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

func databricksOAuthIndividualArgs() []string {
	return []string{
		"databricks", "oauth-individual",
		"--name", "db1",
		"--host", "dbc-xxxx.cloud.databricks.com",
		"--http-path", "/sql/1.0/warehouses/abc123",
		"--catalog", "main",
		"--schema", "default",
		"--client-id", "cid",
		"--client-secret", "csec",
	}
}

func runDatabricksOAuthIndividual(t *testing.T, deps Deps, args []string, stdin string) (*bytes.Buffer, error) {
	t.Helper()
	g := newCreateGroup(deps)
	stdio, out, _ := testcli.NewIO(strings.NewReader(stdin))
	return out, g.Run(context.Background(), args, stdio)
}

func TestCreateDatabricksOAuthIndividualHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 99.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	out, err := runDatabricksOAuthIndividual(t, f.deps(), databricksOAuthIndividualArgs(), "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "connectorId: 99") || !strings.Contains(s, "lazily at first query") {
		t.Errorf("stdout=%q", s)
	}
	req := string(f.lastRawReq)
	for _, want := range []string{
		`"connectorType":"DATABRICKS"`, `"authStrategy":"per_member_oauth"`,
		`"oauthU2m":`, `"clientId":"cid"`, `"clientSecret":"csec"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("req missing %s in %s", want, req)
		}
	}
	for _, unwanted := range []string{`"pat":`, `"clientCredentials":`, `"oauthSso":`, `"token":`} {
		if strings.Contains(req, unwanted) {
			t.Errorf("req unexpectedly contains %s in %s", unwanted, req)
		}
	}
}

func TestCreateDatabricksOAuthIndividualSecretStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	args := []string{
		"databricks", "oauth-individual",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	_, err := runDatabricksOAuthIndividual(t, f.deps(), args, "stdin-secret\n")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"clientSecret":"stdin-secret"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestCreateDatabricksOAuthIndividualSecretStdinEmpty(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-individual",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid", "--client-secret-stdin",
	}
	_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthIndividualSecretStdinReadErr(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-individual",
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

func TestCreateDatabricksOAuthIndividualMissingSecret(t *testing.T) {
	t.Parallel()
	args := []string{
		"databricks", "oauth-individual",
		"--name", "db1", "--host", "h", "--http-path", "/p",
		"--catalog", "c", "--schema", "s",
		"--client-id", "cid",
	}
	_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthIndividualMissingFlags(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), []string{"databricks", "oauth-individual"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthIndividualEmptyString(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"name", "host", "http-path", "catalog", "schema", "client-id"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			args := append(databricksOAuthIndividualArgs(), "--"+flag, "")
			_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), args, "")
			if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "--"+flag) {
				t.Errorf("flag=%s err=%v", flag, err)
			}
		})
	}
}

func TestCreateDatabricksOAuthIndividualJSONBypass(t *testing.T) {
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
	if err := g.Run(ctx, databricksOAuthIndividualArgs(), stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connectorId\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestCreateDatabricksOAuthIndividualRenderWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": 1.0, "name": "db1", "connectorType": "DATABRICKS"}
			return nil
		},
	}
	g := newCreateGroup(f.deps())
	err := g.Run(context.Background(), databricksOAuthIndividualArgs(), testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v want boom", err)
	}
}

// TestCreateDatabricksOAuthIndividualRejectsExtraPositionals pins the
// no-positional contract for the deeply-nested leaf: trailing tokens after
// the verb path must yield ErrUsage before RequireFlags or any RPC fires.
func TestCreateDatabricksOAuthIndividualRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), []string{"databricks", "oauth-individual", "extra"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestCreateDatabricksOAuthIndividualBadFlag(t *testing.T) {
	t.Parallel()
	_, err := runDatabricksOAuthIndividual(t, (&fakeDeps{}).deps(), []string{"databricks", "oauth-individual", "--nope"}, "")
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthIndividualUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	_, err := runDatabricksOAuthIndividual(t, f.deps(), databricksOAuthIndividualArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestCreateDatabricksOAuthIndividualRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connectorId": "not-an-int"}
			return nil
		},
	}
	_, err := runDatabricksOAuthIndividual(t, f.deps(), databricksOAuthIndividualArgs(), "")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
