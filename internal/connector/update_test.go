package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestUpdateHappyPartial(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			// Pre-fetch inherits connectorType + full postgres block; server
			// requires both on every UpdateConnector call (captured 400).
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				out.Connector.Name = "old-name"
				out.Connector.PostgresMetadata = postgresSpec{
					Host: "oldhost", Port: 5432, User: "olduser",
					Database: "olddb", SSLMode: false,
				}
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0, "name": "renamed"}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--name", "renamed", "1"}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "name:") {
		t.Errorf("stdout=%q", out.String())
	}
	// Wire body (final UpdateConnector call): connectorId top-level, renamed
	// config.name, inherited connectorType + full postgres block.
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":1`) {
		t.Errorf("connectorId must be top-level: %s", req)
	}
	if !strings.Contains(req, `"name":"renamed"`) {
		t.Errorf("config.name missing: %s", req)
	}
	if !strings.Contains(req, `"connectorType":"POSTGRES"`) {
		t.Errorf("connectorType must be inherited from GetConnector: %s", req)
	}
	// Server rejects updates without the full postgres block even when only
	// --name changes — baseline values must be forwarded as-is.
	if !strings.Contains(req, `"host":"oldhost"`) || !strings.Contains(req, `"port":5432`) {
		t.Errorf("postgres baseline must be inherited: %s", req)
	}
	// Password isn't returned by GetConnector — must not leak into the body.
	if strings.Contains(req, `"password":`) {
		t.Errorf("password must be omitted when user didn't touch it: %s", req)
	}
}

// Regression: positional <id> placed BEFORE flags must not drop the trailing
// flags (stdlib fs.Parse stops at the first non-flag token). We fixed this by
// routing every verb through cli.ParseFlags, but 100% branch coverage on that
// helper did not catch the verb-level regression we hit in prod — so each verb
// that takes positional+flag gets an explicit ordering test.
func TestUpdateRegressionPositionalBeforeFlags(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				out.Connector.Name = "old"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 9.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	// Positional FIRST (the ordering that broke profile add in prod).
	args := []string{"9", "--name", "renamed", "--host", "h2"}
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":9`) {
		t.Errorf("id lost: %s", req)
	}
	if !strings.Contains(req, `"name":"renamed"`) {
		t.Errorf("--name dropped when placed after positional: %s", req)
	}
	if !strings.Contains(req, `"host":"h2"`) {
		t.Errorf("--host dropped when placed after positional: %s", req)
	}
}

func TestUpdateDialectFields(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--host", "newhost", "--port", "6543", "--ssl=true", "7"}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"connectorId":7`) {
		t.Errorf("req=%s", req)
	}
	if !strings.Contains(req, `"connectorType":"POSTGRES"`) {
		t.Errorf("dialect touched → connectorType required: %s", req)
	}
	if !strings.Contains(req, `"host":"newhost"`) || !strings.Contains(req, `"port":6543`) {
		t.Errorf("req=%s", req)
	}
	if !strings.Contains(req, `"sslMode":true`) {
		t.Errorf("sslMode missing: %s", req)
	}
}

func TestUpdatePasswordStdin(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--password-stdin", "42"}
	stdio, _, _ := testcli.NewIO(strings.NewReader("new-pw\n"))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"new-pw"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdatePasswordFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	args := []string{"--password", "inline-pw", "42"}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"password":"inline-pw"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdatePasswordStdinReadErr(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(errReader{err: errors.New("read fail")})
	err := cmd.Run(context.Background(), []string{"--password-stdin", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateTypeAloneSetsConnectorType(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--type", "postgres", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorType":"POSTGRES"`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestUpdateUserAndDatabaseOnly(t *testing.T) {
	t.Parallel()
	// Exercises the --user / --database flag-visited branches that the dialect
	// tests above didn't touch.
	f := &fakeDeps{}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	args := []string{"--user", "newu", "--database", "newdb", "1"}
	if err := cmd.Run(context.Background(), args, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	req := string(f.lastRawReq)
	if !strings.Contains(req, `"user":"newu"`) || !strings.Contains(req, `"database":"newdb"`) {
		t.Errorf("req=%s", req)
	}
}

func TestUpdateWrongType(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--type", "mysql", "1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateNonIntPositional(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateNoFieldsProvided(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &updateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope", "1"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateUnaryErr(t *testing.T) {
	t.Parallel()
	// GetConnector pre-fetch fails — the "fetch current" branch.
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "fetch current") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateCallErr(t *testing.T) {
	t.Parallel()
	// GetConnector succeeds; UpdateConnector itself errors — separate branch.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			return errors.New("update-boom")
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "update-boom") {
		t.Errorf("err=%v", err)
	}
}

func TestUpdateJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"--name", "n", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"connector\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestUpdateNoConnectorKey(t *testing.T) {
	t.Parallel()
	// Response without "connector" — falls back to raw JSON dump.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				out := resp.(*getConnectorResp)
				out.Connector.ConnectorType = "POSTGRES"
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"weird": 1.0}
			return nil
		},
	}
	cmd := &updateCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"--name", "n", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"weird\"") {
		t.Errorf("stdout=%q", out.String())
	}
}
