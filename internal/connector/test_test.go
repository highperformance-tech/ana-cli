package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// stubGetConnector returns a minimal GetConnector response so `connector test`
// can build a config body for the follow-up TestConnector call.
func stubGetConnector(resp any) {
	out := resp.(*map[string]any)
	*out = map[string]any{
		"connector": map[string]any{
			"id":               1.0,
			"name":             "probe",
			"connectorType":    "POSTGRES",
			"postgresMetadata": map[string]any{"host": "h", "port": 5432.0, "user": "u", "database": "d", "dialect": "postgres", "sslMode": true},
		},
	}
}

func TestTestOK(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				stubGetConnector(resp)
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"error": ""}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "OK" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != servicePath+"/TestConnector" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestTestFail(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				stubGetConnector(resp)
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"error": "connection refused"}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "FAIL") || !strings.Contains(out.String(), "connection refused") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTestJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				stubGetConnector(resp)
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"error": ""}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"error\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTestMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestNonInt(t *testing.T) {
	t.Parallel()
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestTestBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"test", "1", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				stubGetConnector(resp)
				return nil
			}
			out := resp.(*map[string]any)
			*out = map[string]any{"error": 123.0}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestTestTestConnectorUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if strings.HasSuffix(path, "/GetConnector") {
				stubGetConnector(resp)
				return nil
			}
			return errors.New("boom")
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestTestMissingConnectorType(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"connector": map[string]any{"id": 1.0}}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "missing connectorType") {
		t.Errorf("err=%v", err)
	}
}

func TestTestMissingConnector(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &testCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "missing connector object") {
		t.Errorf("err=%v", err)
	}
}
