package e2e

import (
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

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
