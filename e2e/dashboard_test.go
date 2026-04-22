package e2e

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// firstDashboardID asks `dashboard list --json` for at least one id, returning
// empty when the org has no dashboards (in which case the caller should skip).
// Reused by every read-only dashboard leaf that needs a known-good id without
// forcing an env var onto operators who happen to have any dashboard already.
func firstDashboardID(t *testing.T, h *harness.H) string {
	t.Helper()
	raw, err := h.RunJSON("dashboard", "list")
	if err != nil {
		t.Fatalf("dashboard list --json: %v", err)
	}
	arr, _ := raw["dashboards"].([]any)
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		if id, _ := entry["id"].(string); id != "" {
			return id
		}
	}
	return ""
}

// TestDashboardList exercises the default table render. No skip gate: an empty
// org still renders the header row.
func TestDashboardList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("dashboard", "list")
	if err != nil {
		t.Fatalf("dashboard list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("dashboard list missing table header: %s", out)
	}
}

// TestDashboardListJSON asserts the --json mode emits a parseable object
// containing a `dashboards` array. Shape contract — value drift is OK.
func TestDashboardListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("dashboard", "list")
	if err != nil {
		t.Fatalf("dashboard list --json: %v", err)
	}
	if _, ok := raw["dashboards"]; !ok {
		t.Errorf("--json response missing `dashboards` key: %v", raw)
	}
}

// TestDashboardFoldersList renders the ID/NAME folders table.
func TestDashboardFoldersList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("dashboard", "folders", "list")
	if err != nil {
		t.Fatalf("dashboard folders list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("dashboard folders list missing table header: %s", out)
	}
}

// TestDashboardFoldersListJSON covers --json on the folders leaf.
func TestDashboardFoldersListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	stdout, stderr, err := h.Run("--json", "dashboard", "folders", "list")
	if err != nil {
		t.Fatalf("dashboard folders list --json: %v\nstderr: %s", err, stderr)
	}
	if stdout == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("folders list --json: not valid JSON: %v\nstdout=%q", err, stdout)
	}
}

// TestDashboardGet uses the first id surfaced by list (skip if the org has no
// dashboards) and asserts the default summary renders id/name fields.
func TestDashboardGet(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstDashboardID(t, h)
	if id == "" {
		t.Skip("e2e: no dashboards in org; skipping dashboard get")
	}
	out, stderr, err := h.Run("dashboard", "get", id)
	if err != nil {
		t.Fatalf("dashboard get %s: %v\nstderr: %s", id, err, stderr)
	}
	if !strings.Contains(out, id) {
		t.Errorf("dashboard get output missing id %q: %s", id, out)
	}
}

// TestDashboardGetJSON covers the --json render path.
func TestDashboardGetJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstDashboardID(t, h)
	if id == "" {
		t.Skip("e2e: no dashboards in org; skipping dashboard get --json")
	}
	raw, err := h.RunJSON("dashboard", "get", id)
	if err != nil {
		t.Fatalf("dashboard get %s --json: %v", id, err)
	}
	dash, ok := raw["dashboard"].(map[string]any)
	if !ok {
		t.Fatalf("--json response missing `dashboard` object: %v", raw)
	}
	if gotID, _ := dash["id"].(string); gotID != id {
		t.Errorf("dashboard.id = %q, want %q", gotID, id)
	}
}

// TestDashboardHealth exercises the runtime health check for an existing
// dashboard. Requires ANA_E2E_DASHBOARD_ID because an arbitrary dashboard may
// never have been spawned — in which case the server returns a non-contract
// error rather than a health row — and we don't want a flaky assertion.
//
// TODO(e2e): create the dashboard ephemerally inside the test (chat with
// dashboardMode=true + "publish a sin(x) dashboard" prompt + parse id from
// the stream output), register a deferred delete, and drop the
// ANA_E2E_DASHBOARD_ID env var entirely. Blocked on: CLI doesn't expose
// `chat new --dashboard-mode`, and DashboardService/Delete* isn't captured
// in api-catalog yet — need to identify the real delete endpoint so the
// harness can cascade-clean the dashboard the chat publishes. Until that
// lands, this test stays env-gated and skips when the var is unset.
func TestDashboardHealth(t *testing.T) {
	id := os.Getenv("ANA_E2E_DASHBOARD_ID")
	if id == "" {
		t.Skip("e2e: ANA_E2E_DASHBOARD_ID not set; skipping dashboard health")
	}
	h := harness.Begin(t)
	out, stderr, err := h.Run("dashboard", "health", id)
	if err != nil {
		t.Fatalf("dashboard health %s: %v\nstderr: %s", id, err, stderr)
	}
	if h.DryRun() {
		return
	}
	// Output is "<id> HEALTHY" or "<id> UNHEALTHY: <msg>" — both acceptable.
	// Assert the id is echoed rather than the specific health label so this
	// test doesn't flake when the runtime happens to be down.
	if !strings.Contains(out, id) {
		t.Errorf("dashboard health output missing id %q: %s", id, out)
	}
}

// TestDashboardSpawn asks the server to (re)spawn a dashboard runtime. Does
// not create a new dashboard row — spawn just refreshes the runtime for an
// existing dashboard — so no ledger cleanup is needed. Requires an explicit
// env-gated id since spawning touches billed runtime quotas.
//
// TODO(e2e): same as TestDashboardHealth — replace the env-gated id with an
// ephemeral chat-published dashboard + deferred delete once the delete
// endpoint is captured. Leaving dashboards in the org is the wrong pattern;
// every e2e resource should be fully self-contained.
func TestDashboardSpawn(t *testing.T) {
	id := os.Getenv("ANA_E2E_DASHBOARD_ID")
	if id == "" {
		t.Skip("e2e: ANA_E2E_DASHBOARD_ID not set; skipping dashboard spawn")
	}
	h := harness.Begin(t)
	out, stderr, err := h.Run("dashboard", "spawn", id)
	if err != nil {
		t.Fatalf("dashboard spawn %s: %v\nstderr: %s", id, err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "spawned "+id) && !strings.Contains(out, "refreshedAt") {
		t.Errorf("dashboard spawn output should reference id or refreshedAt: %s", out)
	}
}
