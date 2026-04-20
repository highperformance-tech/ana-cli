package feed

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

func TestShowTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != feedServicePath+"/GetFeed" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"posts": []any{
					map[string]any{
						"id": "p1", "title": "KPIs",
						"creatorAgentName": "Org Agent",
						"upvoteCount":      2,
						"createdAt":        "2026-03-31T22:08:16Z",
					},
					// Second post exercises the default-dash branches for
					// title, agent, and created timestamp.
					map[string]any{"id": "p2"},
				},
			}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{"ID", "TITLE", "AGENT", "UPVOTES", "CREATED", "p1", "KPIs", "Org Agent", "p2"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: `strings.Contains(s, "-")` would pass
	// trivially because the p1 row contains an RFC3339 timestamp full of
	// hyphens. Instead, locate the p2 row (id-only fixture, everything
	// else unset) and confirm TITLE, AGENT, and CREATED render as "-";
	// UPVOTES defaults to the integer zero ("0"), not "-".
	foundP2Row := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[0] == "p2" &&
			fields[1] == "-" && fields[2] == "-" && fields[3] == "0" && fields[4] == "-" {
			foundP2Row = true
			break
		}
	}
	if !foundP2Row {
		t.Errorf("expected p2 row to render TITLE/AGENT/CREATED as '-' with UPVOTES='0': %q", s)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

func TestShowJSON(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": []any{}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("stdout=%q should start with {", out.String())
	}
	if !strings.Contains(out.String(), "\"posts\"") {
		t.Errorf("stdout=%q", out.String())
	}
}

func TestShowUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "feed show") {
		t.Errorf("err=%v", err)
	}
}

func TestShowBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestShowRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": "nope"}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestShowJSONEncodeErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"posts": []any{}}
			return nil
		},
	}
	cmd := &showCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
