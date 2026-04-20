package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/highperformance-tech/ana-cli/internal/cli"
	"github.com/highperformance-tech/ana-cli/internal/testcli"
)

// --- tail: happy path / table ---

func TestTailTable(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, path string, _, resp any) error {
			if path != auditServicePath+"/ListAuditLogs" {
				t.Errorf("path=%s", path)
			}
			out := resp.(*map[string]any)
			*out = map[string]any{
				"entries": []any{
					map[string]any{
						"actorEmail":   "brad@example.com",
						"action":       "api_key.created",
						"resourceType": "api_key",
						"resourceId":   "f8e934f5",
						"createdAt":    "2026-04-17T15:19:15Z",
					},
					// resourceId-only (no type).
					map[string]any{
						"actorEmail": "ken@example.com",
						"action":     "auth.login_success",
						"resourceId": "r1",
						"createdAt":  "2026-04-14T20:04:35Z",
					},
					// No resource at all — target should be "-".
					map[string]any{
						"actorEmail": "ken@example.com",
						"action":     "auth.logout",
						"createdAt":  "2026-04-14T20:05:00Z",
					},
					// Everything missing — all default to "-".
					map[string]any{},
				},
			}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	s := out.String()
	for _, want := range []string{
		"TIME", "ACTOR", "ACTION", "TARGET",
		"brad@example.com", "api_key.created", "api_key:f8e934f5",
		"ken@example.com", "auth.login_success", "r1",
		"auth.logout",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output %q", want, s)
		}
	}
	// Row-specific dash assertion: `strings.Contains(s, "-")` trivially
	// passes because the TIME column already contains hyphens (RFC3339
	// `2026-04-17...`). Instead, locate the `auth.logout` row — that row's
	// record has no resourceId/resourceType, so the TARGET cell (rightmost
	// column) must be rendered as the "-" placeholder.
	foundLogoutDash := false
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.Contains(line, "auth.logout") && strings.HasSuffix(strings.TrimSpace(line), "-") {
			foundLogoutDash = true
			break
		}
	}
	if !foundLogoutDash {
		t.Errorf("expected auth.logout row to end with '-' TARGET placeholder: %q", s)
	}
	// No flags -> body must be exactly {}.
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

// --- tail: --since with fake clock ---

func TestTailSinceSetsRFC3339(t *testing.T) {
	t.Parallel()
	// A fixed non-UTC instant. The code converts to UTC before formatting so
	// we can assert the exact wire string regardless of the local zone.
	f := &fakeDeps{now: time.Date(2026, 4, 17, 10, 0, 0, 0, time.FixedZone("EST", -5*3600))}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"--since", "1h"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	wantSince := f.now.Add(-time.Hour).UTC().Format(time.RFC3339)
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	if got["since"] != wantSince {
		t.Errorf("since=%v want %q", got["since"], wantSince)
	}
	if _, ok := got["limit"]; ok {
		t.Errorf("body should not contain limit when --limit is unset: %s", f.lastRawReq)
	}
}

// TestTailRejectsNegativeSince guards the "from the future" bug where
// `time.ParseDuration("-1h")` silently turned into `Now().Add(+1h)`, masking an
// operator typo. Negative durations must now be rejected at parse time with a
// usage error — same posture as --limit.
func TestTailRejectsNegativeSince(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--since", "-1h"}, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if !strings.Contains(err.Error(), "since") {
		t.Errorf("err=%q should mention since", err.Error())
	}
	if f.lastRawReq != nil {
		t.Errorf("Unary should not be called on invalid --since: raw=%s", f.lastRawReq)
	}
}

func TestTailSinceInvalid(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--since", "nope"}, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	// The user-facing message should hint at both accepted forms so the
	// operator does not have to re-read --help.
	if !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("err=%q should mention RFC3339", err.Error())
	}
}

// TestTailSinceRFC3339 verifies an absolute timestamp is accepted and
// re-emitted in UTC RFC3339 form. The clock is irrelevant here — the input
// itself carries the wall-clock instant.
func TestTailSinceRFC3339(t *testing.T) {
	t.Parallel()
	// Now should NOT be consulted for the absolute path; using the zero
	// time would surface any mistaken `Now().Add(...)` arithmetic.
	f := &fakeDeps{now: time.Time{}}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	in := "2026-04-18T05:30:00-04:00"
	if err := cmd.Run(context.Background(), []string{"--since", in}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	want := "2026-04-18T09:30:00Z" // same instant, normalised to UTC.
	if got["since"] != want {
		t.Errorf("since=%v want %q", got["since"], want)
	}
}

// TestTailSinceFractional verifies that callers can paste timestamps copied
// from logs (which often carry fractional seconds) without truncating them.
// Go's time.RFC3339 parser is permissive enough to accept the nano form, and
// the formatted output drops the fractional part to match the duration
// branch's wire shape.
func TestTailSinceFractional(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	in := "2026-04-18T09:30:00.123456789Z"
	if err := cmd.Run(context.Background(), []string{"--since", in}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	want := "2026-04-18T09:30:00Z"
	if got["since"] != want {
		t.Errorf("since=%v want %q", got["since"], want)
	}
}

// --- tail: --limit pass-through ---

func TestTailLimitZeroOmitted(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"--limit", "0"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if string(f.lastRawReq) != "{}" {
		t.Errorf("req=%s want {}", string(f.lastRawReq))
	}
}

// TestTailRejectsNegativeLimit guards the user-visible input-validation bug
// where a negative --limit would silently drop off the wire (via `omitempty`)
// and behave like "unspecified". Negative values are now rejected at parse
// time with a usage error so the operator sees the mistake immediately.
func TestTailRejectsNegativeLimit(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--limit", "-1"}, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
	if !strings.Contains(err.Error(), "--limit") {
		t.Errorf("err=%q should mention --limit", err.Error())
	}
	// Ensure we never dispatched the RPC on an invalid input.
	if f.lastRawReq != nil {
		t.Errorf("Unary should not be called on invalid --limit: raw=%s", f.lastRawReq)
	}
}

func TestTailLimitSet(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), []string{"--limit", "50"}, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(f.lastRawReq, &got); err != nil {
		t.Fatalf("unmarshal err=%v raw=%s", err, f.lastRawReq)
	}
	// JSON numbers decode to float64.
	if v, ok := got["limit"].(float64); !ok || int(v) != 50 {
		t.Errorf("limit=%v want 50", got["limit"])
	}
}

// --- tail: no --since -> body omits "since" ---

func TestTailNoSinceOmitsKey(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	if err := cmd.Run(context.Background(), nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(string(f.lastRawReq), "since") {
		t.Errorf("req should omit since: %s", f.lastRawReq)
	}
}

// --- tail: --json ---

// TestTailJSONEmpty: zero entries means zero JSONL lines (not even an empty
// envelope). Verifies the per-record loop produces no output when the response
// has no entries.
func TestTailJSONEmpty(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": []any{}}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout=%q want empty for zero-entry JSONL", out.String())
	}
}

// TestTailJSONLines: multi-entry response should produce one JSON object per
// line, no envelope wrapping. Each line must independently parse as JSON and
// the final byte must be a newline so downstream tools can append.
func TestTailJSONLines(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{
				"entries": []any{
					map[string]any{
						"actorEmail": "brad@example.com",
						"action":     "api_key.created",
						"createdAt":  "2026-04-17T15:19:15Z",
					},
					map[string]any{
						"actorEmail": "ken@example.com",
						"action":     "auth.login_success",
						"createdAt":  "2026-04-14T20:04:35Z",
					},
				},
			}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio, out, _ := testcli.NewIO(nil)
	if err := cmd.Run(ctx, nil, stdio); err != nil {
		t.Fatalf("err=%v", err)
	}
	raw := out.String()
	// Must NOT be a pretty envelope: the old behaviour wrapped everything in
	// `{ "entries": [...] }`. JSONL must not contain that key.
	if strings.Contains(raw, "\"entries\"") {
		t.Errorf("stdout=%q should not contain envelope key", raw)
	}
	if !strings.HasSuffix(raw, "\n") {
		t.Errorf("stdout=%q should end with newline", raw)
	}
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), raw)
	}
	for i, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Errorf("line %d not valid JSON (%v): %q", i, err, line)
		}
	}
	// Spot-check that the per-record fields are present (not the envelope).
	if !strings.Contains(lines[0], "brad@example.com") {
		t.Errorf("line 0 missing actorEmail: %q", lines[0])
	}
	if !strings.Contains(lines[1], "auth.login_success") {
		t.Errorf("line 1 missing action: %q", lines[1])
	}
}

// --- tail: error paths ---

func TestTailUnaryErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	f := &fakeDeps{unaryFn: func(_ context.Context, _ string, _, _ any) error { return boom }}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err=%v want wrap of boom", err)
	}
	if !strings.Contains(err.Error(), "audit tail") {
		t.Errorf("err=%v should prefix with command name", err)
	}
}

func TestTailBadFlag(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), []string{"--nope"}, stdio)
	if !errors.Is(err, cli.ErrUsage) {
		t.Errorf("err=%v want ErrUsage", err)
	}
}

func TestTailRemarshalErr(t *testing.T) {
	t.Parallel()
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": "nope"}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	stdio, _, _ := testcli.NewIO(nil)
	err := cmd.Run(context.Background(), nil, stdio)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("err=%v", err)
	}
}

func TestTailJSONEncodeErr(t *testing.T) {
	t.Parallel()
	// Entries must be non-empty: encoding now happens per-record, so a zero-
	// entry response would skip the encoder entirely and the failingWriter
	// would never fire.
	f := &fakeDeps{
		unaryFn: func(_ context.Context, _ string, _, resp any) error {
			out := resp.(*map[string]any)
			*out = map[string]any{"entries": []any{
				map[string]any{"actorEmail": "brad@example.com"},
			}}
			return nil
		},
	}
	cmd := &tailCmd{deps: f.deps()}
	ctx := cli.WithGlobal(context.Background(), cli.Global{JSON: true})
	stdio := testcli.FailingIO()
	if err := cmd.Run(ctx, nil, stdio); err == nil || !strings.Contains(err.Error(), "w boom") {
		t.Errorf("err=%v", err)
	}
}
