package connector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestTablesHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"tables": []any{
					map[string]any{"tableSchema": "demo", "tableName": "trips"},
					map[string]any{"tableSchema": "demo", "tableName": "stations"},
				},
			}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(context.Background(), []string{"70"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "SCHEMA") || !strings.Contains(s, "NAME") {
		t.Errorf("headers missing: %q", s)
	}
	if !strings.Contains(s, "trips") || !strings.Contains(s, "stations") {
		t.Errorf("rows missing: %q", s)
	}
	if f.lastPath != servicePath+"/ListConnectorTables" {
		t.Errorf("path=%s", f.lastPath)
	}
	if !strings.Contains(string(f.lastRawReq), `"connectorId":70`) {
		t.Errorf("req=%s", string(f.lastRawReq))
	}
}

func TestTablesJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"tables": []any{}}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(strings.NewReader(""))
	if err := cmd.Run(ctx, []string{"1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"tables\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestTablesMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesNonInt(t *testing.T) {
	t.Parallel()
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &tablesCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestTablesUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestTablesRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"tables": "nope"}
			return nil
		},
	}
	cmd := &tablesCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(strings.NewReader(""))
	err := cmd.Run(context.Background(), []string{"1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
