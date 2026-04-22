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

// TestOrgShowJSON asserts --json returns an envelope keyed by `organization`.
func TestOrgShowJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("org", "show")
	if err != nil {
		t.Fatalf("org show --json: %v", err)
	}
	if _, ok := raw["organization"]; !ok {
		// Some servers have wrapped this under `org` historically — accept
		// either so the smoke doesn't flake on a minor rename.
		if _, alt := raw["org"]; !alt {
			t.Errorf("--json response missing organization envelope: %v", raw)
		}
	}
}

// TestOrgListJSON asserts --json emits an `organizations` array.
func TestOrgListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("org", "list")
	if err != nil {
		t.Fatalf("org list --json: %v", err)
	}
	if _, ok := raw["organizations"]; !ok {
		t.Errorf("--json response missing `organizations` key: %v", raw)
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

// TestOrgMembersListJSON asserts --json emits a `members` key.
func TestOrgMembersListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("org", "members", "list")
	if err != nil {
		t.Fatalf("org members list --json: %v", err)
	}
	if _, ok := raw["members"]; !ok {
		t.Errorf("--json response missing `members` key: %v", raw)
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

// TestOrgRolesList renders ID/NAME for RBAC roles.
func TestOrgRolesList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("org", "roles", "list")
	if err != nil {
		t.Fatalf("org roles list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("org roles list missing header: %s", out)
	}
}

// TestOrgRolesListJSON asserts --json emits a `roles` key.
func TestOrgRolesListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("org", "roles", "list")
	if err != nil {
		t.Fatalf("org roles list --json: %v", err)
	}
	if _, ok := raw["roles"]; !ok {
		t.Errorf("--json response missing `roles` key: %v", raw)
	}
}

// TestOrgPermissionsList renders the ID/NAME table for RBAC permissions.
// This surface is a static catalog for the org, so the header + at least
// one row are expected in any real org.
func TestOrgPermissionsList(t *testing.T) {
	h := harness.Begin(t)
	out, stderr, err := h.Run("org", "permissions", "list")
	if err != nil {
		t.Fatalf("org permissions list: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Errorf("org permissions list missing header: %s", out)
	}
}

// TestOrgPermissionsListJSON asserts --json emits a `permissions` key.
func TestOrgPermissionsListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("org", "permissions", "list")
	if err != nil {
		t.Fatalf("org permissions list --json: %v", err)
	}
	if _, ok := raw["permissions"]; !ok {
		t.Errorf("--json response missing `permissions` key: %v", raw)
	}
}
