package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- rename ---------------------------------------------------------------

func TestRenameHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &renameCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-x", "new title"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "ok" {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	if !strings.Contains(req, `"chatId":"chat-x"`) || !strings.Contains(req, `"summary":"new title"`) {
		t.Errorf("req=%s", req)
	}
	if f.lastPath != chatServicePath+"/UpdateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestRenameMissingID(t *testing.T) {
	t.Parallel()
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameMissingTitle(t *testing.T) {
	t.Parallel()
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-x"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameEmptyTitle(t *testing.T) {
	t.Parallel()
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"chat-x", ""}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestRenameBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	// Include the required <id> <title> positionals so missing-arg isn't the
	// failure path; this isolates the unknown-flag (--nope) parse error.
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"rename", "chat-x", "new title", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err=%v want unknown-flag ErrUsage", err)
	}
}

func TestRenameUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &renameCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x", "t"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestRenameJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &renameCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x", "t"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestRenameRejectsExtraPositionals exercises the `len(args) > 2` branch:
// trailing tokens beyond `<id> <title>` must yield ErrUsage so the operator
// quotes multi-word titles instead of having them silently dropped.
func TestRenameRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &renameCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "title", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

// --- bookmark / unbookmark -----------------------------------------------

func TestBookmarkHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &bookmarkCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "ok" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/BookmarkChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestBookmarkMissingID(t *testing.T) {
	t.Parallel()
	cmd := &bookmarkCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestBookmarkRejectsExtraPositionals exercises the simpleAck strict-arity
// branch: bookmark/unbookmark share a helper, so this also covers
// simpleAck's `len(args) > 1` rejection path for both verbs.
func TestBookmarkRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &bookmarkCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestBookmarkBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	// Include the required <id> positional so missing-arg isn't the failure
	// path; this isolates the unknown-flag (--nope) parse error.
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"bookmark", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err=%v want unknown-flag ErrUsage", err)
	}
}

func TestBookmarkUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &bookmarkCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestBookmarkJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"foo": 1.0}
			return nil
		},
	}
	cmd := &bookmarkCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"foo\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestUnbookmarkHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &unbookmarkCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if f.lastPath != chatServicePath+"/UnbookmarkChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestUnbookmarkMissingID(t *testing.T) {
	t.Parallel()
	cmd := &unbookmarkCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestUnbookmarkBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	// Include the required <id> positional so missing-arg isn't the failure
	// path; this isolates the unknown-flag (--nope) parse error.
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"unbookmark", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err=%v want unknown-flag ErrUsage", err)
	}
}

func TestUnbookmarkUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &unbookmarkCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestUnbookmarkJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"foo": 1.0}
			return nil
		},
	}
	cmd := &unbookmarkCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"foo\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- delete ---------------------------------------------------------------

func TestDeleteHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"chat-7"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "deleted chat-7") {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/DeleteChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDeleteMissingID(t *testing.T) {
	t.Parallel()
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestDeleteRejectsExtraPositionals pins the strict-arity contract for
// `chat delete`: trailing tokens beyond the single <id> must yield ErrUsage.
func TestDeleteRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestDeleteBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	// Include the required <id> positional so missing-arg isn't the failure
	// path; this isolates the unknown-flag (--nope) parse error.
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"delete", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err=%v want unknown-flag ErrUsage", err)
	}
}

func TestDeleteUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &deleteCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{}
			return nil
		},
	}
	cmd := &deleteCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "{}") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- duplicate ------------------------------------------------------------

func TestDuplicateHappy(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "dup-id"}}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"src"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "dup-id" {
		t.Errorf("stdout=%q", out.String())
	}
	if f.lastPath != chatServicePath+"/DuplicateChat" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestDuplicateMissingID(t *testing.T) {
	t.Parallel()
	cmd := &duplicateCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

// TestDuplicateRejectsExtraPositionals pins the strict-arity contract for
// `chat duplicate`: trailing tokens beyond the single <id> must yield
// ErrUsage.
func TestDuplicateRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Errorf("err=%v want strict-arity ErrUsage", err)
	}
	if f.lastPath != "" {
		t.Errorf("Unary should not be called on positional-arity failure: path=%q", f.lastPath)
	}
}

func TestDuplicateBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	// Include the required <id> positional so missing-arg isn't the failure
	// path; this isolates the unknown-flag (--nope) parse error.
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"duplicate", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Errorf("err=%v want unknown-flag ErrUsage", err)
	}
}

func TestDuplicateUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": "not-an-object"}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestDuplicateJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"chat": map[string]any{"id": "x"}}
			return nil
		},
	}
	cmd := &duplicateCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"x"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"chat\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// --- write-failure paths --------------------------------------------------

func TestRenameWriteErr(t *testing.T) {
	t.Parallel()
	cmd := &renameCmd{deps: (&fakeDeps{}).deps()}
	err := cmd.Run(context.Background(), []string{"x", "t"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "chat rename") {
		t.Errorf("err=%v", err)
	}
}

func TestDeleteWriteErr(t *testing.T) {
	t.Parallel()
	cmd := &deleteCmd{deps: (&fakeDeps{}).deps()}
	err := cmd.Run(context.Background(), []string{"x"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "chat delete") {
		t.Errorf("err=%v", err)
	}
}

func TestBookmarkWriteErr(t *testing.T) {
	t.Parallel()
	cmd := &bookmarkCmd{deps: (&fakeDeps{}).deps()}
	err := cmd.Run(context.Background(), []string{"x"}, testcli.FailingIO())
	if err == nil || !strings.Contains(err.Error(), "chat bookmark") {
		t.Errorf("err=%v", err)
	}
}
