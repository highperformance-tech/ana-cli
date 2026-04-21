package e2e

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// connectorId: <int> is the first line of non-JSON stdout from every
// snowflake create leaf; this regex extracts the id so the test can register
// cleanup and assert on the value.
var snowflakeConnectorIDRE = regexp.MustCompile(`(?m)^connectorId:\s+(\d+)\s*$`)

// sfCommonEnv holds the Snowflake connection fields every auth mode shares.
// Empty optional fields are preserved so tests can exercise omitempty paths
// (warehouse/schema/role).
type sfCommonEnv struct {
	locator   string
	database  string
	warehouse string
	schema    string
	role      string
}

// snowflakeCommonEnvOrSkip reads the mode-agnostic ANA_E2E_SF_* env vars and
// skips the calling test if any required field (LOCATOR, DATABASE) is empty.
// Snowflake auth shapes make unreachable defaults meaningless — unlike
// Postgres, the server does more up-front validation — so each Snowflake test
// skips outright when its env is absent rather than submitting a made-up spec.
func snowflakeCommonEnvOrSkip(t *testing.T) sfCommonEnv {
	t.Helper()
	env := sfCommonEnv{
		locator:   os.Getenv("ANA_E2E_SF_LOCATOR"),
		database:  os.Getenv("ANA_E2E_SF_DATABASE"),
		warehouse: os.Getenv("ANA_E2E_SF_WAREHOUSE"),
		schema:    os.Getenv("ANA_E2E_SF_SCHEMA"),
		role:      os.Getenv("ANA_E2E_SF_ROLE"),
	}
	if env.locator == "" || env.database == "" {
		t.Skip("e2e: ANA_E2E_SF_LOCATOR and ANA_E2E_SF_DATABASE must be set for Snowflake tests")
	}
	return env
}

// snowflakeCommonArgs returns the --name/--locator/--database (+ optional
// warehouse/schema/role) flags shared by every Snowflake auth-mode leaf.
func snowflakeCommonArgs(h *harness.H, suffix string, env sfCommonEnv) []string {
	args := []string{
		"--name", h.ResourceName(suffix),
		"--locator", env.locator,
		"--database", env.database,
	}
	if env.warehouse != "" {
		args = append(args, "--warehouse", env.warehouse)
	}
	if env.schema != "" {
		args = append(args, "--schema", env.schema)
	}
	if env.role != "" {
		args = append(args, "--role", env.role)
	}
	return args
}

// extractConnectorID pulls the integer id out of `connectorId: <int>` stdout.
// Fails the test if no match — the leaf's contract is to always emit this line
// on success, so a miss means the output shape drifted.
func extractConnectorID(t *testing.T, stdout string) int {
	t.Helper()
	m := snowflakeConnectorIDRE.FindStringSubmatch(stdout)
	if len(m) != 2 {
		t.Fatalf("could not find connectorId in stdout:\n%s", stdout)
	}
	id, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("connectorId %q is not an int: %v", m[1], err)
	}
	return id
}

// TestConnectorCreateSnowflakePassword smokes `connector create snowflake
// password --password-stdin`. Requires ANA_E2E_SF_USER + ANA_E2E_SF_PASSWORD
// in addition to the common LOCATOR/DATABASE pair.
func TestConnectorCreateSnowflakePassword(t *testing.T) {
	common := snowflakeCommonEnvOrSkip(t)
	user := os.Getenv("ANA_E2E_SF_USER")
	password := os.Getenv("ANA_E2E_SF_PASSWORD")
	if user == "" || password == "" {
		t.Skip("e2e: ANA_E2E_SF_USER and ANA_E2E_SF_PASSWORD required for Snowflake password mode")
	}

	h := harness.Begin(t)
	args := append([]string{"connector", "create", "snowflake", "password"},
		snowflakeCommonArgs(h, "sf-password", common)...)
	args = append(args, "--user", user, "--password-stdin")

	stdout, stderr, err := h.RunStdin(password+"\n", args...)
	if err != nil {
		t.Fatalf("connector create snowflake password: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: SNOWFLAKE") {
		t.Errorf("stdout missing connectorType: SNOWFLAKE:\n%s", stdout)
	}
	// Verify the server can read the new row back.
	if _, estderr, gerr := h.Run("connector", "get", fmt.Sprint(id)); gerr != nil {
		t.Fatalf("connector get %d: %v\nstderr: %s", id, gerr, estderr)
	}
}

// TestConnectorCreateSnowflakeKeypair smokes the keypair leaf. Requires
// ANA_E2E_SF_USER + ANA_E2E_SF_PRIVATE_KEY_PATH (optional passphrase env via
// --private-key-passphrase-stdin).
func TestConnectorCreateSnowflakeKeypair(t *testing.T) {
	common := snowflakeCommonEnvOrSkip(t)
	user := os.Getenv("ANA_E2E_SF_USER")
	keyPath := os.Getenv("ANA_E2E_SF_PRIVATE_KEY_PATH")
	if user == "" || keyPath == "" {
		t.Skip("e2e: ANA_E2E_SF_USER and ANA_E2E_SF_PRIVATE_KEY_PATH required for Snowflake keypair mode")
	}
	passphrase := os.Getenv("ANA_E2E_SF_PRIVATE_KEY_PASSPHRASE")

	h := harness.Begin(t)
	args := append([]string{"connector", "create", "snowflake", "keypair"},
		snowflakeCommonArgs(h, "sf-keypair", common)...)
	args = append(args, "--user", user, "--private-key-file", keyPath)
	stdin := ""
	if passphrase != "" {
		args = append(args, "--private-key-passphrase-stdin")
		stdin = passphrase + "\n"
	}

	stdout, stderr, err := h.RunStdin(stdin, args...)
	if err != nil {
		t.Fatalf("connector create snowflake keypair: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: SNOWFLAKE") {
		t.Errorf("stdout missing connectorType: SNOWFLAKE:\n%s", stdout)
	}
}

// TestConnectorCreateSnowflakeOAuthSSO smokes the oauth-sso leaf. Asserts the
// success note references the configured endpoint — that's the fix that
// shipped alongside the leaf and is the only per-leaf-unique line in stdout.
func TestConnectorCreateSnowflakeOAuthSSO(t *testing.T) {
	common := snowflakeCommonEnvOrSkip(t)
	clientID := os.Getenv("ANA_E2E_SF_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("ANA_E2E_SF_OAUTH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("e2e: ANA_E2E_SF_OAUTH_CLIENT_ID and ANA_E2E_SF_OAUTH_CLIENT_SECRET required for Snowflake oauth-sso mode")
	}

	h := harness.Begin(t)
	args := append([]string{"connector", "create", "snowflake", "oauth-sso"},
		snowflakeCommonArgs(h, "sf-oauth-sso", common)...)
	args = append(args, "--oauth-client-id", clientID, "--oauth-client-secret-stdin")

	stdout, stderr, err := h.RunStdin(clientSecret+"\n", args...)
	if err != nil {
		t.Fatalf("connector create snowflake oauth-sso: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: SNOWFLAKE") {
		t.Errorf("stdout missing connectorType: SNOWFLAKE:\n%s", stdout)
	}
	endpoint := os.Getenv("ANA_E2E_ENDPOINT")
	if endpoint == "" {
		t.Fatalf("ANA_E2E_ENDPOINT should be set inside Begin — got empty")
	}
	if !strings.Contains(stdout, "complete OAuth at "+endpoint) {
		t.Errorf("oauth-sso note should reference ANA_E2E_ENDPOINT %q:\n%s", endpoint, stdout)
	}
}

// TestConnectorCreateSnowflakeOAuthIndividual smokes the oauth-individual
// leaf. Asserts the per-member-lazy note since that's the only leaf-unique
// piece of stdout.
func TestConnectorCreateSnowflakeOAuthIndividual(t *testing.T) {
	common := snowflakeCommonEnvOrSkip(t)
	clientID := os.Getenv("ANA_E2E_SF_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("ANA_E2E_SF_OAUTH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("e2e: ANA_E2E_SF_OAUTH_CLIENT_ID and ANA_E2E_SF_OAUTH_CLIENT_SECRET required for Snowflake oauth-individual mode")
	}

	h := harness.Begin(t)
	args := append([]string{"connector", "create", "snowflake", "oauth-individual"},
		snowflakeCommonArgs(h, "sf-oauth-individual", common)...)
	args = append(args, "--oauth-client-id", clientID, "--oauth-client-secret-stdin")

	stdout, stderr, err := h.RunStdin(clientSecret+"\n", args...)
	if err != nil {
		t.Fatalf("connector create snowflake oauth-individual: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: SNOWFLAKE") {
		t.Errorf("stdout missing connectorType: SNOWFLAKE:\n%s", stdout)
	}
	if !strings.Contains(stdout, "lazily at first query") {
		t.Errorf("oauth-individual note should mention lazy per-member auth:\n%s", stdout)
	}
}
