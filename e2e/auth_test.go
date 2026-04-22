package e2e

import (
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// apiKeyIDByName walks `auth keys list --json` and returns the id of the
// row whose name matches. Empty return means "not found" — callers treat
// that as a fatal test error.
func apiKeyIDByName(t *testing.T, h *harness.H, name string) string {
	t.Helper()
	raw, err := h.RunJSON("auth", "keys", "list")
	if err != nil {
		t.Fatalf("auth keys list --json: %v", err)
	}
	arr, _ := raw["apiKeys"].([]any)
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		if n, _ := entry["name"].(string); n == name {
			id, _ := entry["id"].(string)
			return id
		}
	}
	return ""
}

// TestAuthKeysListCreateRevoke is the Tier-1 golden chain for API keys. The
// harness creates a test-prefixed key and defers the revoke; the body
// verifies ListApiKeys sees it and that it survives JSON round-trip.
func TestAuthKeysListCreateRevoke(t *testing.T) {
	h := harness.Begin(t)
	k := h.CreateAPIKey("keys")
	if k.ID == "" && !h.DryRun() {
		t.Fatalf("CreateAPIKey returned empty id")
	}
	if k.Token == "" && !h.DryRun() {
		t.Errorf("CreateAPIKey returned empty plaintext token")
	}
	out, stderr, err := h.Run("auth", "keys", "list")
	if err != nil {
		t.Fatalf("auth keys list: %v\nstderr: %s", err, stderr)
	}
	if !h.DryRun() && !strings.Contains(out, h.ResourceName("keys")) {
		t.Errorf("new key not in list output: %s", out)
	}
}

// TestAuthKeysRotate exercises the create -> rotate -> revoke chain that the
// plan calls out as "formerly Tier-3". The deferred revoke from CreateAPIKey
// is redirected to the rotated id inside RotateAPIKey, so no ledger entry is
// needed — parent-nesting absorbs the irreversibility.
func TestAuthKeysRotate(t *testing.T) {
	h := harness.Begin(t)
	k := h.CreateAPIKey("rotate")
	if k.ID == "" {
		if h.DryRun() {
			return
		}
		t.Fatalf("CreateAPIKey returned empty id")
	}
	rotated := h.RotateAPIKey(k.ID)
	if rotated.ID == k.ID {
		t.Errorf("rotate returned same id %s as original", k.ID)
	}
	if rotated.Token == "" {
		t.Errorf("rotate returned empty plaintext")
	}
}

// TestAuthServiceAccountsCreateDelete mirrors the keys golden chain for the
// service-accounts surface.
func TestAuthServiceAccountsCreateDelete(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateServiceAccount("sa")
	if id == "" && !h.DryRun() {
		t.Fatalf("CreateServiceAccount returned empty memberId")
	}
	out, stderr, err := h.Run("auth", "service-accounts", "list")
	if err != nil {
		t.Fatalf("service-accounts list: %v\nstderr: %s", err, stderr)
	}
	if !h.DryRun() && !strings.Contains(out, h.ResourceName("sa")) {
		t.Errorf("new service account not in list: %s", out)
	}
}

// TestAuthKeysListJSON asserts --json emits an `apiKeys` array.
func TestAuthKeysListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("auth", "keys", "list")
	if err != nil {
		t.Fatalf("auth keys list --json: %v", err)
	}
	if _, ok := raw["apiKeys"]; !ok {
		t.Errorf("--json response missing `apiKeys` key: %v", raw)
	}
}

// TestAuthKeysCreateCLI drives `auth keys create` + `revoke` straight through
// the CLI. The id is recovered from `list --json` since the create verb
// intentionally only emits the plaintext token — see auth/keys.go's
// emitPlaintextToken.
func TestAuthKeysCreateCLI(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		_, _, _ = h.Run("auth", "keys", "create", "--name", h.ResourceName("cli-key"))
		return
	}
	name := h.ResourceName("cli-key")
	// Pre-register the name-based safety net so a successful create followed
	// by a failing list-lookup or assertion can't leak a real API key.
	h.RegisterAPIKeyCleanupByName(name)
	stdout, stderr, err := h.Run("auth", "keys", "create", "--name", name)
	if err != nil {
		t.Fatalf("auth keys create: %v\nstderr: %s", err, stderr)
	}
	plaintext := strings.TrimSpace(stdout)
	if plaintext == "" {
		t.Errorf("auth keys create: stdout empty (expected plaintext token)")
	}
	// Secret: do not echo plaintext back via t.Logf. The reminder we assert
	// on goes to stderr.
	if !strings.Contains(stderr, "will not be shown again") {
		t.Errorf("auth keys create: missing 'will not be shown again' reminder on stderr: %s", stderr)
	}
	id := apiKeyIDByName(t, h, name)
	if id == "" {
		t.Fatalf("auth keys create: could not find %q in keys list", name)
	}
	h.Register(func() {
		_, revErr, err := h.Run("auth", "keys", "revoke", id)
		if err != nil {
			h.RecordManualRevert("api_key:"+id, "revoke failed: "+err.Error()+" stderr="+revErr)
		}
	})
}

// TestAuthKeysRotateCLI drives the create -> rotate -> revoke chain through
// the CLI. The original id is rotated server-side (old revoked automatically)
// so cleanup targets the rotated id. The rotate call emits a fresh plaintext
// which must not be logged.
func TestAuthKeysRotateCLI(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	name := h.ResourceName("cli-rotate")
	// Same-name safety net works across the rotate: the rotated key keeps
	// the logical name, so list-by-name will find whichever id is current.
	h.RegisterAPIKeyCleanupByName(name)
	stdout, _, err := h.Run("auth", "keys", "create", "--name", name)
	if err != nil {
		t.Fatalf("auth keys create (for rotate): %v", err)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("create returned empty plaintext")
	}
	origID := apiKeyIDByName(t, h, name)
	if origID == "" {
		t.Fatalf("could not find created key %q in list", name)
	}
	rotateOut, rotateErr, err := h.Run("auth", "keys", "rotate", origID)
	if err != nil {
		t.Fatalf("auth keys rotate: %v\nstderr: %s", err, rotateErr)
	}
	if strings.TrimSpace(rotateOut) == "" {
		t.Errorf("rotate returned empty plaintext")
	}
	if !strings.Contains(rotateErr, "will not be shown again") {
		t.Errorf("rotate missing reminder on stderr: %s", rotateErr)
	}
	// The rotated key takes the same logical name, so look it up again.
	newID := apiKeyIDByName(t, h, name)
	if newID == "" {
		t.Fatalf("could not find rotated key %q in list", name)
	}
	if newID == origID {
		t.Errorf("rotate returned same id %q as original", origID)
	}
	h.Register(func() {
		_, revErr, err := h.Run("auth", "keys", "revoke", newID)
		if err != nil {
			h.RecordManualRevert("api_key:"+newID, "post-rotate revoke failed: "+err.Error()+" stderr="+revErr)
		}
	})
}

// TestAuthServiceAccountsCreateCLI drives `service-accounts create` +
// `delete` through the CLI. The create command prints `<memberId> <name>`
// so we can parse the id directly without hitting --json.
func TestAuthServiceAccountsCreateCLI(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	name := h.ResourceName("cli-sa")
	h.RegisterServiceAccountCleanupByName(name)
	stdout, stderr, err := h.Run("auth", "service-accounts", "create",
		"--name", name, "--description", "e2e temp")
	if err != nil {
		t.Fatalf("service-accounts create: %v\nstderr: %s", err, stderr)
	}
	fields := strings.Fields(strings.TrimSpace(stdout))
	if len(fields) < 2 {
		t.Fatalf("service-accounts create: expected '<memberId> <name>', got %q", stdout)
	}
	id := fields[0]
	h.Register(func() {
		_, delErr, err := h.Run("auth", "service-accounts", "delete", id)
		if err != nil {
			h.RecordManualRevert("service_account:"+id, "delete failed: "+err.Error()+" stderr="+delErr)
		}
	})
	listOut, _, err := h.Run("auth", "service-accounts", "list")
	if err != nil {
		t.Fatalf("service-accounts list (post-create): %v", err)
	}
	if !strings.Contains(listOut, id) {
		t.Errorf("service-accounts list missing new id %q: %s", id, listOut)
	}
}

// TestAuthServiceAccountsListJSON asserts --json emits `serviceAccounts`.
func TestAuthServiceAccountsListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("auth", "service-accounts", "list")
	if err != nil {
		t.Fatalf("service-accounts list --json: %v", err)
	}
	if _, ok := raw["serviceAccounts"]; !ok {
		t.Errorf("--json response missing `serviceAccounts` key: %v", raw)
	}
}

// TestAuthKeysCreateMissingName asserts the usage-error path when --name
// is absent.
func TestAuthKeysCreateMissingName(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, stderr, err := h.Run("auth", "keys", "create")
	if err == nil {
		t.Fatalf("expected error for missing --name; stderr=%s", stderr)
	}
	if !strings.Contains(err.Error()+stderr, "name") {
		t.Errorf("expected --name complaint; got err=%v stderr=%q", err, stderr)
	}
}

// TestAuthKeysRotateExtraArg asserts the "exactly one <id>" guard fires.
func TestAuthKeysRotateExtraArg(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, stderr, err := h.Run("auth", "keys", "rotate", "one", "two")
	if err == nil {
		t.Fatalf("expected usage error for extra arg; stderr=%s", stderr)
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("expected 'exactly one' in error; got %v", err)
	}
}
