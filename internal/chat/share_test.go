package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestShareHappyURL(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"share": map[string]any{"shareToken": "TKN"},
				"url":   "https://app/chat/abc?ref=TKN",
			}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "https://app/chat/abc?ref=TKN") {
		t.Errorf("stdout=%q", out.String())
	}
	req := string(f.lastRaw)
	for _, w := range []string{
		`"primitiveId":"abc"`, `"primitiveType":"PRIMITIVE_TYPE_CHAT"`,
		`"channel":"SHARE_CHANNEL_LINK_COPY"`,
	} {
		if !strings.Contains(req, w) {
			t.Errorf("req missing %q: %s", w, req)
		}
	}
	if f.lastPath != sharingServicePath+"/CreateShare" {
		t.Errorf("path=%s", f.lastPath)
	}
}

func TestShareTokenFallback(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": map[string]any{"shareToken": "TKN"}}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.TrimSpace(out.String()) != "TKN" {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShareJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": map[string]any{"shareToken": "T"}}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, []string{"abc"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(out.String(), "\"share\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

// TestShareRejectsExtraPositionals pins the strict-arity contract: trailing
// tokens beyond the single <id> must yield ErrUsage before the RPC fires.
func TestShareRejectsExtraPositionals(t *testing.T) {
	t.Parallel()
	cmd := &shareCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"id1", "extra"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestShareMissingID(t *testing.T) {
	t.Parallel()
	cmd := &shareCmd{deps: (&fakeDeps{}).deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShareBadFlag(t *testing.T) {
	t.Parallel()
	stdio, _, _ := testcli.NewIO(nil)
	err := New((&fakeDeps{}).deps()).Run(context.Background(), []string{"share", "chat-x", "--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v", err)
	}
}

func TestShareUnaryErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return errors.New("boom") }}
	cmd := &shareCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err=%v", err)
	}
}

func TestShareRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"share": "not-an-object"}
			return nil
		},
	}
	cmd := &shareCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"x"}, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}
