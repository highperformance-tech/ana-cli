package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestNewHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "new-id"}}
			return nil
		},
	}
	stdio, out, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(context.Background(), []string{"new", "--connector", "1,2", "--title", "hi"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "new-id" {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	for _, w := range []string{
		`"connectorIds":[1,2]`, `"type":"TYPE_UNIVERSAL"`, `"version":1`,
		`"model":"MODEL_DEFAULT"`, `"methodology":"METHODOLOGY_ADAPTIVE"`,
		`"summary":"hi"`,
	} {
		if !strings.Contains(req, w) {
			t.Errorf("req missing %s: %s", w, req)
		}
	}
	if f.lastPath != chatServicePath+"/CreateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestNewOmitTitleWhenEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return nil }}
	stdio, _, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(context.Background(), []string{"new", "--connector", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRaw), `"summary"`) {
		t.Errorf("summary should be omitted when empty: %s", f.lastRaw)
	}
}

func TestNewJSONBypass(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := New(f.deps()).Run(ctx, []string{"new", "--connector", "1"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestNewMissingConnector(t *testing.T) {
	t.Parallel()
	cmd := &newCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewBadConnector(t *testing.T) {
	t.Parallel()
	cmd := &newCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--connector", "abc"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"new", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestNewUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"new", "--connector", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestNewRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	stdio, _, _ := testcli.NewIO(nil)
	err := New(f.deps()).Run(context.Background(), []string{"new", "--connector", "1"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
