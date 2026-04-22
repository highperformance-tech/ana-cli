package e2e

import (
	"fmt"
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
	args := append([]string{"connector", "create", "databricks", "access-token"},
		databricksCommonArgs(h, "dbx-access-token", common)...)
	args = append(args, "--token-stdin")

	stdout, stderr, err := h.RunStdin(token+"\n", args...)
	if err != nil {
		t.Fatalf("connector create databricks access-token: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: DATABRICKS") {
		t.Errorf("stdout missing connectorType: DATABRICKS:\n%s", stdout)
	}
	if _, estderr, gerr := h.Run("connector", "get", fmt.Sprint(id)); gerr != nil {
		t.Fatalf("connector get %d: %v\nstderr: %s", id, gerr, estderr)
	}
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
	args := append([]string{"connector", "create", "databricks", "client-credentials"},
		databricksCommonArgs(h, "dbx-client-credentials", common)...)
	args = append(args, "--client-id", clientID, "--client-secret-stdin")

	stdout, stderr, err := h.RunStdin(clientSecret+"\n", args...)
	if err != nil {
		t.Fatalf("connector create databricks client-credentials: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: DATABRICKS") {
		t.Errorf("stdout missing connectorType: DATABRICKS:\n%s", stdout)
	}
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
	args := append([]string{"connector", "create", "databricks", "oauth-sso"},
		databricksCommonArgs(h, "dbx-oauth-sso", common)...)
	args = append(args, "--client-id", clientID, "--client-secret-stdin")

	stdout, stderr, err := h.RunStdin(clientSecret+"\n", args...)
	if err != nil {
		t.Fatalf("connector create databricks oauth-sso: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: DATABRICKS") {
		t.Errorf("stdout missing connectorType: DATABRICKS:\n%s", stdout)
	}
	endpoint := h.Endpoint()
	if !strings.Contains(stdout, "complete OAuth at "+endpoint) {
		t.Errorf("oauth-sso note should reference harness endpoint %q:\n%s", endpoint, stdout)
	}
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
	args := append([]string{"connector", "create", "databricks", "oauth-individual"},
		databricksCommonArgs(h, "dbx-oauth-individual", common)...)
	args = append(args, "--client-id", clientID, "--client-secret-stdin")

	stdout, stderr, err := h.RunStdin(clientSecret+"\n", args...)
	if err != nil {
		t.Fatalf("connector create databricks oauth-individual: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	id := extractConnectorID(t, stdout)
	h.RegisterConnectorCleanup(id)
	if !strings.Contains(stdout, "connectorType: DATABRICKS") {
		t.Errorf("stdout missing connectorType: DATABRICKS:\n%s", stdout)
	}
	if !strings.Contains(stdout, "lazily at first query") {
		t.Errorf("oauth-individual note should mention lazy per-member auth:\n%s", stdout)
	}
}
