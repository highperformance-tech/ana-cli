package e2e

import (
	"strconv"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// firstOntologyID pulls an id out of `ontology list --json`. Ontology ids are
// integers on the wire (see internal/ontology/get.go), so we marshal the
// number back to a string for the CLI call. Empty return = skip.
func firstOntologyID(t *testing.T, h *harness.H) string {
	t.Helper()
	raw, err := h.RunJSON("ontology", "list")
	if err != nil {
		t.Fatalf("ontology list --json: %v", err)
	}
	arr, _ := raw["ontologies"].([]any)
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		// JSON numbers decode as float64.
		if n, ok := entry["id"].(float64); ok && n != 0 {
			return strconv.FormatInt(int64(n), 10)
		}
	}
	return ""
}

// TestOntologyList renders ID/NAME. Empty orgs still emit the header.
func TestOntologyList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("ontology", "list")
	if err != nil {
		t.Fatalf("ontology list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("ontology list missing header: %s", out)
	}
}

// TestOntologyListJSON asserts the --json envelope has an `ontologies` key.
func TestOntologyListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("ontology", "list")
	if err != nil {
		t.Fatalf("ontology list --json: %v", err)
	}
	if _, ok := raw["ontologies"]; !ok {
		t.Errorf("--json response missing `ontologies` key: %v", raw)
	}
}

// TestOntologyGet pulls an id from list and asserts the summary echoes it.
// Skips when the org has no ontologies. Note: the command rejects
// non-integer ids at dispatch time (UsageErrf), so we always pass the raw
// integer as captured from list.
func TestOntologyGet(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstOntologyID(t, h)
	if id == "" {
		t.Skip("e2e: no ontologies in org; skipping ontology get")
	}
	out, stderr, err := h.Run("ontology", "get", id)
	if err != nil {
		t.Fatalf("ontology get %s: %v\nstderr: %s", id, err, stderr)
	}
	if !strings.Contains(out, id) {
		t.Errorf("ontology get output missing id %q: %s", id, out)
	}
}

// TestOntologyGetJSON asserts --json returns an `ontology` object whose `id`
// (integer-typed) matches the requested id.
func TestOntologyGetJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	id := firstOntologyID(t, h)
	if id == "" {
		t.Skip("e2e: no ontologies in org; skipping ontology get --json")
	}
	raw, err := h.RunJSON("ontology", "get", id)
	if err != nil {
		t.Fatalf("ontology get %s --json: %v", id, err)
	}
	onto, ok := raw["ontology"].(map[string]any)
	if !ok {
		t.Fatalf("--json response missing `ontology` object: %v", raw)
	}
	got, _ := onto["id"].(float64)
	if strconv.FormatInt(int64(got), 10) != id {
		t.Errorf("ontology.id = %v, want %q", onto["id"], id)
	}
}
