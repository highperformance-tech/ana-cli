package e2e

import (
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// TestOrgShow is the first read-only smoke test. It proves the harness wiring
// end-to-end: env -> transport -> dispatch -> verb -> real server.
func TestOrgShow(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("org", "show")
	if err != nil {
		t.Fatalf("org show: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, h.ExpectOrgID()) {
		t.Errorf("expected org %q in `org show` output; got %q", h.ExpectOrgID(), out)
	}
}

// TestOrgList asserts the token's member sees at least the expected org.
func TestOrgList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("org", "list")
	if err != nil {
		t.Fatalf("org list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, h.ExpectOrgID()) {
		t.Errorf("`org list` missing %q: %s", h.ExpectOrgID(), out)
	}
}

// TestOrgMembersList exercises the nested members verb. No assertion on
// specific emails — just that the RPC round-trips and renders a header.
func TestOrgMembersList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("org", "members", "list")
	if err != nil {
		t.Fatalf("org members list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if out == "" {
		t.Errorf("`org members list` produced no output (stderr=%s)", stderr)
	}
}

// TestWhoami asserts the token resolves to an org that matches EXPECT_ORG.
// The guard in Begin already enforces this; the test covers the whoami verb
// render path separately so drift in GetMember shape fails loudly.
func TestWhoami(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("auth", "whoami")
	if err != nil {
		t.Fatalf("auth whoami: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, h.ExpectOrgID()) {
		t.Errorf("whoami missing org %q: %s", h.ExpectOrgID(), out)
	}
	if !strings.Contains(out, "email") {
		t.Errorf("whoami missing email row: %s", out)
	}
}
