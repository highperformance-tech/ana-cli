package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

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

// snowflakeLeafArgs builds the full argv for `connector create snowflake
// <auth-mode>` using the shared connection flags. `suffix` seeds the
// name-based cleanup safety-net; `extra` carries the auth-mode-specific flags.
func snowflakeLeafArgs(h *harness.H, authMode, suffix string, env sfCommonEnv, extra ...string) []string {
	args := append([]string{"connector", "create", "snowflake", authMode},
		snowflakeCommonArgs(h, suffix, env)...)
	return append(args, extra...)
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
	// Pre-register a name-based safety-net cleanup so a successful create
	// followed by a failing extractConnectorID can't orphan the connector.
	h.RegisterConnectorCleanupByName(h.ResourceName("sf-password"))
	connectorCreateLeaf{
		Name:          "snowflake password",
		Args:          snowflakeLeafArgs(h, "password", "sf-password", common, "--user", user, "--password-stdin"),
		Stdin:         password + "\n",
		ConnectorType: "SNOWFLAKE",
	}.Run(t, h)
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
	h.RegisterConnectorCleanupByName(h.ResourceName("sf-keypair"))
	extra := []string{"--user", user, "--private-key-file", keyPath}
	stdin := ""
	if passphrase != "" {
		extra = append(extra, "--private-key-passphrase-stdin")
		stdin = passphrase + "\n"
	}
	connectorCreateLeaf{
		Name:          "snowflake keypair",
		Args:          snowflakeLeafArgs(h, "keypair", "sf-keypair", common, extra...),
		Stdin:         stdin,
		ConnectorType: "SNOWFLAKE",
	}.Run(t, h)
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
	h.RegisterConnectorCleanupByName(h.ResourceName("sf-oauth-sso"))
	endpoint := h.Endpoint()
	connectorCreateLeaf{
		Name:          "snowflake oauth-sso",
		Args:          snowflakeLeafArgs(h, "oauth-sso", "sf-oauth-sso", common, "--oauth-client-id", clientID, "--oauth-client-secret-stdin"),
		Stdin:         clientSecret + "\n",
		ConnectorType: "SNOWFLAKE",
		Extra: func(stdout string) {
			if !strings.Contains(stdout, "complete OAuth at "+endpoint) {
				t.Errorf("oauth-sso note should reference harness endpoint %q:\n%s", endpoint, stdout)
			}
		},
	}.Run(t, h)
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
	h.RegisterConnectorCleanupByName(h.ResourceName("sf-oauth-individual"))
	connectorCreateLeaf{
		Name:          "snowflake oauth-individual",
		Args:          snowflakeLeafArgs(h, "oauth-individual", "sf-oauth-individual", common, "--oauth-client-id", clientID, "--oauth-client-secret-stdin"),
		Stdin:         clientSecret + "\n",
		ConnectorType: "SNOWFLAKE",
		Extra: func(stdout string) {
			if !strings.Contains(stdout, "lazily at first query") {
				t.Errorf("oauth-individual note should mention lazy per-member auth:\n%s", stdout)
			}
		},
	}.Run(t, h)
}
