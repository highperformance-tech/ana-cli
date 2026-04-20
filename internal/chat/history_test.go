package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestHistoryHappy(t *testing.T) {
	t.Parallel()
	// Cover every cell variant branch plus the "other" default.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": []any{
				map[string]any{
					"id": "c1", "timestamp": "t1", "lifecycle": "L",
					"mdCell": map[string]any{"content": "hello\nworld"},
				},
				map[string]any{
					"id": "c2", "timestamp": "t2", "lifecycle": "L",
					"pyCell": map[string]any{"code": "print('hi')"},
				},
				map[string]any{
					"id": "c3", "timestamp": "t3", "lifecycle": "L",
					"statusCell": map[string]any{"status": "working"},
				},
				map[string]any{
					"id": "c4", "timestamp": "t4", "lifecycle": "L",
					"summaryCell": map[string]any{"summary": "done"},
				},
				// No variant → "other".
				map[string]any{"id": "c5", "timestamp": "t5", "lifecycle": "L"},
			}}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, w := range []string{"md: hello", "py: print", "status: working", "summary: done", "other: "} {
		if !strings.Contains(s, w) {
			t.Errorf("missing %q in %q", w, s)
		}
	}
	if !strings.Contains(string(f.lastRaw), `"chatId":"x"`) {
		t.Errorf("req=%s", f.lastRaw)
	}
}

func TestHistoryJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": []any{}}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"cells\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestHistoryMissingPositional(t *testing.T) {
	t.Parallel()
	cmd := &historyCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryBadFlag(t *testing.T) {
	t.Parallel()
	cmd := &historyCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &historyCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryWriteErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": []any{
				map[string]any{"id": "c1", "timestamp": "t", "lifecycle": "L",
					"mdCell": map[string]any{"content": "hi"}},
			}}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	err := cmd.Run(context.Background(), []string{"x"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "chat history") {
		t.Errorf("err=%v", err)
	}
}

func TestHistoryRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"cells": "not-an-array"}
			return nil
		},
	}
	cmd := &historyCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
