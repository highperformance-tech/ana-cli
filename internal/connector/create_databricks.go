package connector

import (
	"flag"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newDatabricksCreateGroup returns the Databricks create-dialect Group.
// Flags common to every Databricks auth-mode leaf are declared on the
// Group's persistent Flags closure; each auth-mode leaf declares its own
// credential-specific flags via Flagger. The resolver merges them onto the
// leaf's parsed FlagSet automatically.
//
// All five workspace fields (`--host`, `--http-path`, `--port`, `--catalog`,
// `--schema`) are required by the TextQL UI across every Databricks auth
// mode, so they live on the Group rather than being duplicated per-leaf.
// `--port` defaults to 443; the CLI accepts any 1..65535 value so
// self-hosted forwarders aren't blocked.
func newDatabricksCreateGroup(deps Deps) *cli.Group {
	var (
		name     string
		host     string
		httpPath string
		port     int
		catalog  string
		schema   string
	)
	return &cli.Group{
		Summary: "Create a Databricks connector. Pick an auth mode.",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&name, "name", "", "connector name (required)")
			fs.StringVar(&host, "host", "", "Databricks workspace hostname without scheme, e.g. dbc-xxxx.cloud.databricks.com (required)")
			fs.StringVar(&httpPath, "http-path", "", "SQL warehouse path, e.g. /sql/1.0/warehouses/abc123 (required)")
			fs.IntVar(&port, "port", 443, "SQL warehouse port (defaults to 443)")
			fs.StringVar(&catalog, "catalog", "", "Unity Catalog name (required)")
			fs.StringVar(&schema, "schema", "", "default schema (required)")
		},
		Children: map[string]cli.Command{
			"access-token": &databricksAccessTokenCmd{
				deps: deps, name: &name, host: &host, httpPath: &httpPath,
				port: &port, catalog: &catalog, schema: &schema,
			},
			"client-credentials": &databricksClientCredentialsCmd{
				deps: deps, name: &name, host: &host, httpPath: &httpPath,
				port: &port, catalog: &catalog, schema: &schema,
			},
			"oauth-sso": &databricksOAuthSSOCmd{
				deps: deps, name: &name, host: &host, httpPath: &httpPath,
				port: &port, catalog: &catalog, schema: &schema,
			},
			"oauth-individual": &databricksOAuthIndividualCmd{
				deps: deps, name: &name, host: &host, httpPath: &httpPath,
				port: &port, catalog: &catalog, schema: &schema,
			},
		},
	}
}

// requireDatabricksCommon enforces non-empty values for the shared ancestor
// flags + validates --port sits in the TCP range. Returned as a helper so
// every Databricks leaf applies the same validation before building its
// request.
func requireDatabricksCommon(prefix, name, host, httpPath string, port int, catalog, schema string) error {
	for _, p := range []struct {
		name, val string
	}{
		{"name", name}, {"host", host}, {"http-path", httpPath},
		{"catalog", catalog}, {"schema", schema},
	} {
		if p.val == "" {
			return cli.UsageErrf("%s: --%s must not be empty", prefix, p.name)
		}
	}
	if port <= 0 || port > 65535 {
		return cli.UsageErrf("%s: --port must be in 1..65535 (got %d)", prefix, port)
	}
	return nil
}

// requireDatabricksClientID guards against `--client-id ""` on every
// Databricks leaf that takes one (client-credentials, oauth-sso,
// oauth-individual). cli.RequireFlags only checks that the flag was
// explicitly set, so a deliberately empty value still slips through — this
// helper rejects it consistently. Split from requireDatabricksCommon
// because the access-token leaf doesn't use --client-id.
func requireDatabricksClientID(prefix, clientID string) error {
	if clientID == "" {
		return cli.UsageErrf("%s: --client-id must not be empty", prefix)
	}
	return nil
}
