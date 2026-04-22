package e2e

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// connectorIDRE extracts `connectorId: <int>` from the first line of non-JSON
// stdout emitted by every `connector create <dialect> <auth-mode>` leaf.
var connectorIDRE = regexp.MustCompile(`(?m)^connectorId:\s+(\d+)\s*$`)

// extractConnectorID pulls the integer id out of `connectorId: <int>` stdout.
// Fails the test if no match — every create leaf's contract is to emit this
// line on success, so a miss means the output shape drifted.
func extractConnectorID(t *testing.T, stdout string) int {
	t.Helper()
	m := connectorIDRE.FindStringSubmatch(stdout)
	if len(m) != 2 {
		t.Fatalf("could not find connectorId in stdout:\n%s", stdout)
	}
	id, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("connectorId %q is not an int: %v", m[1], err)
	}
	return id
}

// connectorCreateLeaf bundles the invariants every connector-create smoke
// shares: run the command, skip post-create assertions in dry-run, extract +
// register the id, assert `connectorType: <DIALECT>`, run any leaf-specific
// stdout checks, then read the row back via `connector get` to confirm the
// server persisted the new connector.
//
// The helper exists so a parity slip (e.g., a new leaf forgetting the `get`
// round-trip) can only happen if a test intentionally bypasses this wrapper.
type connectorCreateLeaf struct {
	// Name is the leaf identifier used in fatal error messages — typically
	// "databricks access-token" or "snowflake oauth-sso".
	Name string
	// Args is the full argv passed to `h.RunStdin`, starting with
	// "connector", "create", <dialect>, <auth-mode>, ...
	Args []string
	// Stdin is the stdin payload for secret flags (token, password, etc.).
	// Empty when no --*-stdin flag is used.
	Stdin string
	// ConnectorType is the dialect tag asserted in stdout, e.g. "DATABRICKS"
	// or "SNOWFLAKE". Matched against the literal `connectorType: <tag>` line.
	ConnectorType string
	// Extra runs after the common assertions and before the `connector get`
	// round-trip. Use it for leaf-unique stdout fragments (OAuth endpoint
	// note, per-member-lazy note, etc.). May be nil.
	Extra func(stdout string)
}

// Run executes the leaf smoke. On non-dry-run success, the created connector
// id is registered for cleanup and read back via `connector get`. Returns the
// created id so callers can chain additional assertions if needed; in dry-run
// mode the returned id is 0.
func (l connectorCreateLeaf) Run(t *testing.T, h *harness.H) int {
	t.Helper()
	stdout, stderr, err := h.RunStdin(l.Stdin, l.Args...)
	if err != nil {
		t.Fatalf("connector create %s: %v\nstderr: %s", l.Name, err, stderr)
	}
	if h.DryRun() {
		return 0
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	typeLine := "connectorType: " + l.ConnectorType
	if !strings.Contains(stdout, typeLine) {
		t.Errorf("stdout missing %s:\n%s", typeLine, stdout)
	}
	if l.Extra != nil {
		l.Extra(stdout)
	}
	if _, estderr, gerr := h.Run("connector", "get", fmt.Sprint(id)); gerr != nil {
		t.Fatalf("connector get %d: %v\nstderr: %s", id, gerr, estderr)
	}
	return id
}
