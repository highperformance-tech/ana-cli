package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// firstPlaybookID pulls an id out of `playbook list --json`. Empty return
// signals the caller to t.Skip — the readonly leaves have nothing to assert
// against without at least one playbook in the org.
func firstPlaybookID(t *testing.T, h *harness.H) string {
	t.Helper()
	raw, err := h.RunJSON("playbook", "list")
	if err != nil {
		t.Fatalf("playbook list --json: %v", err)
	}
	arr, _ := raw["playbooks"].([]any)
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		if id, _ := entry["id"].(string); id != "" {
			return id
		}
	}
	return ""
}

// TestPlaybookList renders the ID/NAME/SCHEDULE table. Empty orgs still emit
// the header row, so no skip gate.
func TestPlaybookList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("playbook", "list")
	if err != nil {
		t.Fatalf("playbook list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	for _, col := range []string{"ID", "NAME", "SCHEDULE"} {
		if !strings.Contains(out, col) {
			t.Errorf("playbook list missing column %q: %s", col, out)
		}
	}
}

// TestPlaybookListJSON asserts --json emits a `playbooks` array (shape
// contract only — drift in individual row values is expected).
func TestPlaybookListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("playbook", "list")
	if err != nil {
		t.Fatalf("playbook list --json: %v", err)
	}
	if _, ok := raw["playbooks"]; !ok {
		t.Errorf("--json response missing `playbooks` key: %v", raw)
	}
}

// TestPlaybookGet discovers a playbook from list and asserts the summary
// echoes its id. Skips when the org has no playbooks.
func TestPlaybookGet(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook get")
	}
	out, stderr, err := h.Run("playbook", "get", id)
	if err != nil {
		t.Fatalf("playbook get %s: %v\nstderr: %s", id, err, stderr)
	}
	if !strings.Contains(out, id) {
		t.Errorf("playbook get output missing id %q: %s", id, out)
	}
}

// TestPlaybookGetJSON asserts --json returns a `playbook` object with the
// requested id (contract check — server may add fields; we only assert id).
func TestPlaybookGetJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook get --json")
	}
	raw, err := h.RunJSON("playbook", "get", id)
	if err != nil {
		t.Fatalf("playbook get %s --json: %v", id, err)
	}
	pb, ok := raw["playbook"].(map[string]any)
	if !ok {
		t.Fatalf("--json response missing `playbook` object: %v", raw)
	}
	if gotID, _ := pb["id"].(string); gotID != id {
		t.Errorf("playbook.id = %q, want %q", gotID, id)
	}
}

// TestPlaybookReports exercises the RUN_ID/SUBJECT/RAN_AT render. A playbook
// with zero historical runs still emits the header row, so no per-row check
// — the smoke is "the endpoint is reachable and the table flushes".
func TestPlaybookReports(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook reports")
	}
	out, stderr, err := h.Run("playbook", "reports", id)
	if err != nil {
		t.Fatalf("playbook reports %s: %v\nstderr: %s", id, err, stderr)
	}
	for _, col := range []string{"RUN_ID", "SUBJECT", "RAN_AT"} {
		if !strings.Contains(out, col) {
			t.Errorf("playbook reports missing column %q: %s", col, out)
		}
	}
}

// TestPlaybookReportsJSON asserts --json is parseable. The catalog sample
// shows a `reports` key; we assert that loosely (accept empty object too, in
// case the server drops the key when the list is empty).
func TestPlaybookReportsJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook reports --json")
	}
	stdout, stderr, err := h.Run("--json", "playbook", "reports", id)
	if err != nil {
		t.Fatalf("playbook reports %s --json: %v\nstderr: %s", id, err, stderr)
	}
	if stdout == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("reports --json: not valid JSON: %v\nstdout=%q", err, stdout)
	}
}

// TestPlaybookLineage covers the FROM/TO/TYPE table — or the empty-edges
// fallback ("(no lineage edges)") that the command prints when the server
// returns an empty payload. The captured sample in api-catalog is `{}`, so
// accept either output.
func TestPlaybookLineage(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook lineage")
	}
	out, stderr, err := h.Run("playbook", "lineage", id)
	if err != nil {
		t.Fatalf("playbook lineage %s: %v\nstderr: %s", id, err, stderr)
	}
	hasTable := strings.Contains(out, "FROM") && strings.Contains(out, "TO")
	hasEmpty := strings.Contains(out, "(no lineage edges)")
	if !hasTable && !hasEmpty {
		t.Errorf("playbook lineage: neither FROM/TO header nor empty-edges marker: %s", out)
	}
}

// TestPlaybookLineageJSON asserts --json returns valid JSON. An empty body
// is acceptable (catalog sample is literally `{}`).
func TestPlaybookLineageJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstPlaybookID(t, h)
	if id == "" {
		t.Skip("e2e: no playbooks in org; skipping playbook lineage --json")
	}
	stdout, stderr, err := h.Run("--json", "playbook", "lineage", id)
	if err != nil {
		t.Fatalf("playbook lineage %s --json: %v\nstderr: %s", id, err, stderr)
	}
	if stdout == "" {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("lineage --json: not valid JSON: %v\nstdout=%q", err, stdout)
	}
}
