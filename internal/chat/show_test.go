package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestShowHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{
				"id": "xyz", "summary": "s", "model": "m",
				"updatedAt": "u", "source": "src", "methodology": "x",
			}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"xyz"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, w := range []string{"id: xyz", "title: s", "model: m", "updated: u", "source: src", "methodology: x"} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
	if !strings.Contains(string(f.lastRaw), `"chatId":"xyz"`) {
		t.Errorf("req=%s", f.lastRaw)
	}
}

func TestShowJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShowNoChatFallback(t *testing.T) {
	t.Parallel()
	// When `chat` is missing we print raw JSON so the user sees what came back.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"other": 1.0}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"other\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestShowRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestShowRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestShowMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &showCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShowBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"show", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShowUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
