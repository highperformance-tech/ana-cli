package e2e

import (
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// TestProfileLifecycle drives add -> list -> show -> use -> remove through
// `h.Run`/`h.RunStdin`. The harness already seeds a `default` profile into a
// t.TempDir() config (see e2e/harness/harness.go:Begin); this test adds a
// second profile alongside it so the list/use/remove branches get exercised.
//
// Every action goes through cli.Dispatch — no raw config writes — so the
// dispatch path is the one under test even though no RPCs are involved.
func TestProfileLifecycle(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}

	const name = "e2e-temp"
	// 1. add with --token-stdin (primary stdin path; inline --token would
	//    log the value and is deliberately not tested).
	addOut, addErr, err := h.RunStdin("fake-token-value", "profile", "add", name,
		"--endpoint", "https://example.invalid", "--org", "e2e-org", "--token-stdin")
	if err != nil {
		t.Fatalf("profile add: %v\nstderr: %s", err, addErr)
	}
	if !strings.Contains(addOut, "saved profile "+name) {
		t.Errorf("profile add output missing 'saved profile' line: %s", addOut)
	}

	// 2. list: new profile present, default still active.
	listOut, _, err := h.Run("profile", "list")
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}
	if !strings.Contains(listOut, name) {
		t.Errorf("profile list missing new profile %q: %s", name, listOut)
	}
	if !strings.Contains(listOut, "default") {
		t.Errorf("profile list missing seeded default: %s", listOut)
	}

	// 3. list --json: `profiles` envelope + hasToken flag on the new entry.
	raw, err := h.RunJSON("profile", "list")
	if err != nil {
		t.Fatalf("profile list --json: %v", err)
	}
	profiles, _ := raw["profiles"].([]any)
	var found bool
	for _, p := range profiles {
		entry, _ := p.(map[string]any)
		if n, _ := entry["name"].(string); n == name {
			found = true
			if has, _ := entry["hasToken"].(bool); !has {
				t.Errorf("profile %q hasToken = false, want true", name)
			}
		}
	}
	if !found {
		t.Errorf("profile list --json missing new profile %q: %v", name, raw)
	}

	// 4. use: switch the active pointer.
	useOut, _, err := h.Run("profile", "use", name)
	if err != nil {
		t.Fatalf("profile use: %v", err)
	}
	if !strings.Contains(useOut, "active profile: "+name) {
		t.Errorf("profile use output: %s", useOut)
	}

	// 5. show: defaults to the active profile, so should echo the new name.
	showOut, _, err := h.Run("profile", "show")
	if err != nil {
		t.Fatalf("profile show: %v", err)
	}
	if !strings.Contains(showOut, name) {
		t.Errorf("profile show missing %q: %s", name, showOut)
	}

	// 6. show --json: active=true + hasToken=true on the renamed profile.
	showRaw, err := h.RunJSON("profile", "show")
	if err != nil {
		t.Fatalf("profile show --json: %v", err)
	}
	if got, _ := showRaw["name"].(string); got != name {
		t.Errorf("profile show --json name = %q, want %q", got, name)
	}
	if active, _ := showRaw["active"].(bool); !active {
		t.Errorf("profile show --json active = false after use")
	}

	// 7. remove: should also clear the active pointer since we just switched
	//    to the profile we're now deleting.
	rmOut, _, err := h.Run("profile", "remove", name)
	if err != nil {
		t.Fatalf("profile remove: %v", err)
	}
	if !strings.Contains(rmOut, name) {
		t.Errorf("profile remove output missing name: %s", rmOut)
	}

	// 8. list after remove: the profile is gone.
	gone, _, err := h.Run("profile", "list")
	if err != nil {
		t.Fatalf("profile list (post-remove): %v", err)
	}
	if strings.Contains(gone, name) {
		t.Errorf("profile list still contains removed profile %q: %s", name, gone)
	}
}

// TestProfileAddMissingName asserts the usage-error path: dispatch should
// return a cli.ErrUsage when <name> is absent. The harness's exit-code
// mapping surfaces this as a non-nil error with "name is required" in it.
func TestProfileAddMissingName(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, stderr, err := h.RunStdin("tok", "profile", "add", "--token-stdin")
	if err == nil {
		t.Fatalf("profile add with no name should fail; stderr=%s", stderr)
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "name is required") {
		t.Errorf("expected 'name is required' in error; got err=%v stderr=%q", err, stderr)
	}
}

// TestProfileUseUnknown asserts `profile use <missing>` fails with the
// ErrUnknownProfile sentinel surfaced by the dispatch layer.
func TestProfileUseUnknown(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, stderr, err := h.Run("profile", "use", "does-not-exist")
	if err == nil {
		t.Fatalf("profile use <missing> should fail; stderr=%s", stderr)
	}
	if !strings.Contains(err.Error(), "unknown profile") {
		t.Errorf("expected unknown-profile error; got %v", err)
	}
}
