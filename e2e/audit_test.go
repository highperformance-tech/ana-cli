package e2e

import (
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// TestAuditTail exercises ListAuditLogs with a short --since so the response
// stays small. No assertion on specific entries — just "RPC succeeds + output
// is not empty of headers".
func TestAuditTail(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("audit", "tail", "--since", "24h", "--limit", "5")
	if err != nil {
		t.Fatalf("audit tail: %v\nstderr: %s", err, stderr)
	}
	// Table header must be present regardless of whether the org has
	// activity — an empty org still renders `TIME ACTOR ACTION TARGET`.
	if out == "" {
		t.Errorf("audit tail produced no output")
	}
}
