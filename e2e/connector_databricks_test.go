package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// dbxCommonEnv holds the Databricks workspace fields every auth mode shares.
// `port` is optional here because the CLI's --port already defaults to 443;
// only override when the env sets a non-default value.
type dbxCommonEnv struct {
	host     string
	httpPath string
	catalog  string
	schema   string
	port     string
}

// databricksCommonEnvOrSkip reads the mode-agnostic ANA_E2E_DBX_* env vars
// and skips the calling test if any required field (HOST, HTTP_PATH, CATALOG,
// SCHEMA) is empty. Mirrors snowflakeCommonEnvOrSkip — server does up-front
// validation so submitting a made-up spec would drown the suite in noise.
func databricksCommonEnvOrSkip(t *testing.T) dbxCommonEnv {
	t.Helper()
	env := dbxCommonEnv{
		host:     os.Getenv("ANA_E2E_DBX_HOST"),
		httpPath: os.Getenv("ANA_E2E_DBX_HTTP_PATH"),
		catalog:  os.Getenv("ANA_E2E_DBX_CATALOG"),
		schema:   os.Getenv("ANA_E2E_DBX_SCHEMA"),
		port:     os.Getenv("ANA_E2E_DBX_PORT"),
	}
	if env.host == "" || env.httpPath == "" || env.catalog == "" || env.schema == "" {
		t.Skip("e2e: ANA_E2E_DBX_HOST, ANA_E2E_DBX_HTTP_PATH, ANA_E2E_DBX_CATALOG, and ANA_E2E_DBX_SCHEMA must be set for Databricks tests")
	}
	return env
}

// databricksCommonArgs returns the --name/--host/--http-path/--catalog/--schema
// (+ optional --port override) flags shared by every Databricks auth-mode leaf.
func databricksCommonArgs(h *harness.H, suffix string, env dbxCommonEnv) []string {
	args := []string{
		"--name", h.ResourceName(suffix),
		"--host", env.host,
		"--http-path", env.httpPath,
		"--catalog", env.catalog,
		"--schema", env.schema,
	}
	if env.port != "" {
		args = append(args, "--port", env.port)
	}
	return args
}

// databricksLeafArgs builds the full argv for `connector create databricks
// <auth-mode>` using the shared workspace flags. `suffix` seeds the name-based
// cleanup safety-net; `extra` carries the auth-mode-specific flags.
func databricksLeafArgs(h *harness.H, authMode, suffix string, env dbxCommonEnv, extra ...string) []string {
	args := append([]string{"connector", "create", "databricks", authMode},
		databricksCommonArgs(h, suffix, env)...)
	return append(args, extra...)
}

// TestConnectorCreateDatabricksAccessToken smokes
// `connector create databricks access-token --token-stdin`. Requires
// ANA_E2E_DBX_TOKEN in addition to the common workspace env.
func TestConnectorCreateDatabricksAccessToken(t *testing.T) {
	common := databricksCommonEnvOrSkip(t)
	token := os.Getenv("ANA_E2E_DBX_TOKEN")
	if token == "" {
		t.Skip("e2e: ANA_E2E_DBX_TOKEN required for Databricks access-token mode")
	}

	h := harness.Begin(t)
	h.RegisterConnectorCleanupByName(h.ResourceName("dbx-access-token"))
	connectorCreateLeaf{
		Name:          "databricks access-token",
		Args:          databricksLeafArgs(h, "access-token", "dbx-access-token", common, "--token-stdin"),
		Stdin:         token + "\n",
		ConnectorType: "DATABRICKS",
	}.Run(t, h)
}

// TestConnectorCreateDatabricksClientCredentials smokes the M2M leaf.
// Requires ANA_E2E_DBX_CLIENT_ID + ANA_E2E_DBX_CLIENT_SECRET (Service
// Principal applicationId + OAuth secret) alongside the workspace env.
func TestConnectorCreateDatabricksClientCredentials(t *testing.T) {
	common := databricksCommonEnvOrSkip(t)
	clientID := os.Getenv("ANA_E2E_DBX_CLIENT_ID")
	clientSecret := os.Getenv("ANA_E2E_DBX_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("e2e: ANA_E2E_DBX_CLIENT_ID and ANA_E2E_DBX_CLIENT_SECRET required for Databricks client-credentials mode")
	}

	h := harness.Begin(t)
	h.RegisterConnectorCleanupByName(h.ResourceName("dbx-client-credentials"))
	connectorCreateLeaf{
		Name:          "databricks client-credentials",
		Args:          databricksLeafArgs(h, "client-credentials", "dbx-client-credentials", common, "--client-id", clientID, "--client-secret-stdin"),
		Stdin:         clientSecret + "\n",
		ConnectorType: "DATABRICKS",
	}.Run(t, h)
}

// TestConnectorCreateDatabricksOAuthSSO smokes the oauth-sso leaf. Asserts
// the success note references the configured endpoint (matches the Snowflake
// pattern). Requires ANA_E2E_DBX_OAUTH_CLIENT_ID +
// ANA_E2E_DBX_OAUTH_CLIENT_SECRET (Databricks OAuth app credentials, distinct
// from Service Principal credentials used by client-credentials).
func TestConnectorCreateDatabricksOAuthSSO(t *testing.T) {
	common := databricksCommonEnvOrSkip(t)
	clientID := os.Getenv("ANA_E2E_DBX_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("ANA_E2E_DBX_OAUTH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("e2e: ANA_E2E_DBX_OAUTH_CLIENT_ID and ANA_E2E_DBX_OAUTH_CLIENT_SECRET required for Databricks oauth-sso mode")
	}

	h := harness.Begin(t)
	h.RegisterConnectorCleanupByName(h.ResourceName("dbx-oauth-sso"))
	endpoint := h.Endpoint()
	connectorCreateLeaf{
		Name:          "databricks oauth-sso",
		Args:          databricksLeafArgs(h, "oauth-sso", "dbx-oauth-sso", common, "--client-id", clientID, "--client-secret-stdin"),
		Stdin:         clientSecret + "\n",
		ConnectorType: "DATABRICKS",
		Extra: func(stdout string) {
			if !strings.Contains(stdout, "complete OAuth at "+endpoint) {
				t.Errorf("oauth-sso note should reference harness endpoint %q:\n%s", endpoint, stdout)
			}
		},
	}.Run(t, h)
}

// TestConnectorCreateDatabricksOAuthIndividual smokes the oauth-individual
// leaf. Asserts the per-member-lazy note since that's the only leaf-unique
// piece of stdout. Reuses the same ANA_E2E_DBX_OAUTH_CLIENT_* env pair —
// oauth-sso and oauth-individual share the same Databricks OAuth app.
func TestConnectorCreateDatabricksOAuthIndividual(t *testing.T) {
	common := databricksCommonEnvOrSkip(t)
	clientID := os.Getenv("ANA_E2E_DBX_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("ANA_E2E_DBX_OAUTH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("e2e: ANA_E2E_DBX_OAUTH_CLIENT_ID and ANA_E2E_DBX_OAUTH_CLIENT_SECRET required for Databricks oauth-individual mode")
	}

	h := harness.Begin(t)
	h.RegisterConnectorCleanupByName(h.ResourceName("dbx-oauth-individual"))
	connectorCreateLeaf{
		Name:          "databricks oauth-individual",
		Args:          databricksLeafArgs(h, "oauth-individual", "dbx-oauth-individual", common, "--client-id", clientID, "--client-secret-stdin"),
		Stdin:         clientSecret + "\n",
		ConnectorType: "DATABRICKS",
		Extra: func(stdout string) {
			if !strings.Contains(stdout, "lazily at first query") {
				t.Errorf("oauth-individual note should mention lazy per-member auth:\n%s", stdout)
			}
		},
	}.Run(t, h)
}
