package e2e

import (
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// TestFeedShow renders ID/TITLE/AGENT/UPVOTES/CREATED. Empty feed still
// emits the header row, so no skip gate.
func TestFeedShow(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("feed", "show")
	if err != nil {
		t.Fatalf("feed show: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	for _, col := range []string{"ID", "TITLE", "AGENT", "UPVOTES", "CREATED"} {
		if !strings.Contains(out, col) {
			t.Errorf("feed show missing column %q: %s", col, out)
		}
	}
}

// TestFeedShowJSON asserts --json emits a `posts` key.
func TestFeedShowJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("feed", "show")
	if err != nil {
		t.Fatalf("feed show --json: %v", err)
	}
	if _, ok := raw["posts"]; !ok {
		t.Errorf("--json response missing `posts` key: %v", raw)
	}
}

// TestFeedStats renders the key/value counter block. Assert one of the
// counter keys is present so we know the typed render path fired (vs. the
// WriteJSON fallback).
func TestFeedStats(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("feed", "stats")
	if err != nil {
		t.Fatalf("feed stats: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	for _, key := range []string{"messagesToday", "activeAgents", "connectorsConfigured"} {
		if !strings.Contains(out, key) {
			t.Errorf("feed stats missing key %q: %s", key, out)
		}
	}
}

// TestFeedStatsJSON asserts --json is parseable and carries at least one of
// the catalog keys (don't assert exact values — counters drift every tick).
func TestFeedStatsJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("feed", "stats")
	if err != nil {
		t.Fatalf("feed stats --json: %v", err)
	}
	want := []string{
		"messagesToday", "messagesAllTime", "activeAgents",
		"dashboardsCreated", "threadsCreated", "playbooksCreated",
		"connectorsConfigured",
	}
	found := false
	for _, k := range want {
		if _, ok := raw[k]; ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("feed stats --json: none of %v present in %v", want, raw)
	}
}
