package e2e

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// connSpecFromEnv reads optional ANA_E2E_PG_* env vars so the test can use a
// reachable database when one is available. With nothing set, the helpers
// send a syntactically valid but unreachable spec — CreateConnector usually
// accepts it; only `connector test` actually probes the network.
func connSpecFromEnv() harness.ConnSpec {
	port, _ := strconv.Atoi(os.Getenv("ANA_E2E_PG_PORT"))
	if port == 0 {
		port = 5432
	}
	return harness.ConnSpec{
		Host:     envOr("ANA_E2E_PG_HOST", "e2e.invalid"),
		Port:     port,
		User:     envOr("ANA_E2E_PG_USER", "e2e"),
		Password: envOr("ANA_E2E_PG_PASSWORD", "e2e"),
		Database: envOr("ANA_E2E_PG_DATABASE", "postgres"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// TestConnectorList is read-only and checks that the table header renders.
func TestConnectorList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("connector", "list")
	if err != nil {
		t.Fatalf("connector list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("connector list missing table header: %s", out)
	}
}

// TestConnectorCreateDelete is the Tier-1 golden path. Harness registers the
// delete on Begin; the test only has to verify the id comes back.
func TestConnectorCreateDelete(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("create-delete", connSpecFromEnv())
	if id == 0 && !h.DryRun() {
		t.Fatalf("CreateConnector returned id=0")
	}
	// Verify the server can read it back immediately via `get`.
	out, stderr, err := h.Run("connector", "get", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector get %d: %v\nstderr: %s", id, err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, h.ResourceName("create-delete")) {
		t.Errorf("connector get output missing test name: %s", out)
	}
}

// TestConnectorUpdate creates a connector (Tier 1) then mutates its name via
// the CLI, then lets the deferred DeleteConnector clean it up. Because the
// parent is test-owned, this is Tier 1 — not Tier 2 — so no snapshot needed.
func TestConnectorUpdate(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("update", connSpecFromEnv())
	renamed := h.ResourceName("update-renamed")
	_, stderr, err := h.Run("connector", "update", fmt.Sprint(id), "--name", renamed)
	if err != nil {
		t.Fatalf("connector update: %v\nstderr: %s", err, stderr)
	}
	out, stderr, err := h.Run("connector", "get", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector get after update: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, renamed) {
		t.Errorf("update did not take effect; get output: %s", out)
	}
}
