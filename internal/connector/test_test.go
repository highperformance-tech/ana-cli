package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestTestOK(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
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
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
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
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
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
	cmd := &testCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTestRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
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
