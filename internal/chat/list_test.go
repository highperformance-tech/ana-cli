package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestListTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": []any{
				map[string]any{"id": "a", "summary": "first chat", "updatedAt": "2026-04-17"},
				map[string]any{"id": "b", "summary": "second", "updatedAt": "2026-04-16"},
			}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	if !strings.Contains(s, "ID") || !strings.Contains(s, "TITLE") || !strings.Contains(s, "UPDATED") {
		t.Errorf("headers missing: %q", s)
	}
	if !strings.Contains(s, "first chat") || !strings.Contains(s, "second") {
		t.Errorf("rows missing: %q", s)
	}
	if f.lastPath != chatServicePath+"/GetChats" {
		t.Errorf("path=%s", f.lastPath)
	}
	if string(f.lastRaw) != "{}" {
		t.Errorf("body should be empty object: %s", f.lastRaw)
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": []any{}}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chats\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestListUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestListRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chats": "not-an-array"}
			return nil
		},
	}
	cmd := &listCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestListBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &listCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}
