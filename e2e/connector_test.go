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

// TestConnectorListJSON asserts the --json envelope is keyed by `connectors`.
func TestConnectorListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("connector", "list")
	if err != nil {
		t.Fatalf("connector list --json: %v", err)
	}
	if _, ok := raw["connectors"]; !ok {
		t.Errorf("--json response missing `connectors` key: %v", raw)
	}
}

// TestConnectorGetJSON creates a throwaway connector via the harness helper
// and confirms --json returns a `connector` object whose id matches.
func TestConnectorGetJSON(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("get-json", connSpecFromEnv())
	if id == 0 {
		if h.DryRun() {
			return
		}
		t.Fatalf("CreateConnector returned id=0")
	}
	raw, err := h.RunJSON("connector", "get", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector get --json: %v", err)
	}
	conn, ok := raw["connector"].(map[string]any)
	if !ok {
		t.Fatalf("--json response missing `connector` object: %v", raw)
	}
	// id may decode as float64 (generic JSON) — compare numerically.
	if n, _ := conn["id"].(float64); int(n) != id {
		t.Errorf("connector.id = %v, want %d", conn["id"], id)
	}
}

// TestConnectorCreatePostgresCLI drives `connector create postgres password`
// with `--password-stdin`. Uses the shared connectorId regex (defined in
// connector_snowflake_test.go) — the output shape is identical across
// dialects. Explicit --ssl=false is sent so the boolean wire field is
// exercised without requiring TLS on the test target.
func TestConnectorCreatePostgresCLI(t *testing.T) {
	h := harness.Begin(t)
	spec := connSpecFromEnv()
	name := h.ResourceName("pg-cli")
	// Register the name-based safety net BEFORE running the CLI: if the create
	// succeeds server-side but extractConnectorID later fails, the by-name
	// cleanup catches the orphan. The id-based cleanup registered after a
	// successful parse runs first (LIFO), making this a no-op on the happy path.
	h.RegisterConnectorCleanupByName(name)
	args := []string{
		"connector", "create", "postgres", "password",
		"--name", name,
		"--host", spec.Host,
		"--port", fmt.Sprint(spec.Port),
		"--user", spec.User,
		"--database", spec.Database,
		"--password-stdin",
		"--ssl=false",
	}
	stdout, stderr, err := h.RunStdin(spec.Password+"\n", args...)
	if err != nil {
		t.Fatalf("connector create postgres password: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: POSTGRES") {
		t.Errorf("stdout missing connectorType: POSTGRES:\n%s", stdout)
	}
}

// TestConnectorCreatePostgresCLISSL repeats the CLI create with --ssl=true so
// both booleans hit the wire at least once. Cleanup runs LIFO before the
// sibling test's.
func TestConnectorCreatePostgresCLISSL(t *testing.T) {
	h := harness.Begin(t)
	spec := connSpecFromEnv()
	name := h.ResourceName("pg-cli-ssl")
	h.RegisterConnectorCleanupByName(name)
	args := []string{
		"connector", "create", "postgres", "password",
		"--name", name,
		"--host", spec.Host,
		"--port", fmt.Sprint(spec.Port),
		"--user", spec.User,
		"--database", spec.Database,
		"--password-stdin",
		"--ssl",
	}
	stdout, stderr, err := h.RunStdin(spec.Password+"\n", args...)
	if err != nil {
		t.Fatalf("connector create postgres password --ssl: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
}

// TestConnectorUpdatePasswordStdin covers the update --password-stdin path
// — the only update flag combination that reads stdin. A rename happens
// alongside so the test confirms multi-flag updates still merge the
// pre-fetched baseline correctly.
func TestConnectorUpdatePasswordStdin(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("update-pwd", connSpecFromEnv())
	renamed := h.ResourceName("update-pwd-renamed")
	_, stderr, err := h.RunStdin("new-password\n", "connector", "update", fmt.Sprint(id),
		"--name", renamed, "--password-stdin")
	if err != nil {
		t.Fatalf("connector update --password-stdin: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	out, _, err := h.Run("connector", "get", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector get after update: %v", err)
	}
	if !strings.Contains(out, renamed) {
		t.Errorf("rename not applied: %s", out)
	}
}

// TestConnectorTables runs `connector tables <id>`. Requires
// ANA_E2E_PG_HOST because a connector pointing at `e2e.invalid` will time
// out the driver probe rather than surface a clean empty table.
func TestConnectorTables(t *testing.T) {
	if os.Getenv("ANA_E2E_PG_HOST") == "" {
		t.Skip("e2e: ANA_E2E_PG_HOST required for connector tables (driver must reach a real db)")
	}
	h := harness.Begin(t)
	id := h.CreateConnector("tables", connSpecFromEnv())
	out, stderr, err := h.Run("connector", "tables", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector tables: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "SCHEMA") || !strings.Contains(out, "NAME") {
		t.Errorf("connector tables missing header: %s", out)
	}
}

// TestConnectorExamples runs `connector examples <id>` against a throwaway
// connector. The endpoint works even if the db isn't reachable — example
// queries are server-side templates keyed off dialect — so no PG env gate.
func TestConnectorExamples(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("examples", connSpecFromEnv())
	_, stderr, err := h.Run("connector", "examples", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector examples: %v\nstderr: %s", err, stderr)
	}
	_ = stderr
	// Output may be empty for a connector the server hasn't profiled yet;
	// no content assertion — the smoke is "endpoint answers without error".
}

// TestConnectorTest runs `connector test <id>`. Accepts either `OK` or a
// `FAIL:` prefix because the connector points at `e2e.invalid` by default;
// on such a connector the driver probe fails with a real error message,
// which is still a valid contract response (see internal/connector/test.go
// — the command classifies either branch as non-error).
func TestConnectorTest(t *testing.T) {
	h := harness.Begin(t)
	id := h.CreateConnector("test", connSpecFromEnv())
	out, stderr, err := h.Run("connector", "test", fmt.Sprint(id))
	if err != nil {
		t.Fatalf("connector test: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if !strings.Contains(out, "OK") && !strings.Contains(out, "FAIL:") {
		t.Errorf("connector test output should start with OK or FAIL: got %q", out)
	}
}

// TestConnectorGetNotFound covers the typed-error contract for a missing
// connector id. The server returns a Connect-RPC error that the dispatch
// layer surfaces as non-nil; exact message varies, so we only assert that
// the command exits non-zero.
func TestConnectorGetNotFound(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, _, err := h.Run("connector", "get", "999999999")
	if err == nil {
		t.Fatalf("connector get <missing>: expected error, got nil")
	}
}
