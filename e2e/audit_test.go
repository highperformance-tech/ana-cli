package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// TestAuditTail exercises ListAuditLogs with a short --since so the response
// stays small. Assert the table header renders — an empty org still emits
// `TIME ACTOR ACTION TARGET`.
func TestAuditTail(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("audit", "tail", "--since", "24h", "--limit", "5")
	if err != nil {
		t.Fatalf("audit tail: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	for _, col := range []string{"TIME", "ACTOR", "ACTION", "TARGET"} {
		if !strings.Contains(out, col) {
			t.Errorf("audit tail missing header %q: %s", col, out)
		}
	}
}

// TestAuditTailNoSince covers the "since omitted" branch where the wire
// payload drops the field entirely (omitempty). Keep --limit small so the
// response doesn't balloon.
func TestAuditTailNoSince(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("audit", "tail", "--limit", "5")
	if err != nil {
		t.Fatalf("audit tail --limit 5: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "TIME") {
		t.Errorf("audit tail missing TIME header: %s", out)
	}
}

// TestAuditTailJSON asserts the --json path emits JSON Lines. Empty output
// is acceptable (no entries in the last 24h on an idle org). If lines do
// come back, each must round-trip through encoding/json.
func TestAuditTailJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	stdout, stderr, err := h.Run("--json", "audit", "tail", "--since", "24h", "--limit", "5")
	if err != nil {
		t.Fatalf("audit tail --json: %v\nstderr: %s", err, stderr)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return
	}
	for i, line := range strings.Split(stdout, "\n") {
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("audit tail --json line %d not valid JSON: %v\nline=%q", i, err, line)
		}
	}
}
