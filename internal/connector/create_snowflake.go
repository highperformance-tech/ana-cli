package connector

import (
	"flag"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newSnowflakeCreateGroup returns the Snowflake create-dialect Group. Flags
// common to every Snowflake auth-mode leaf are declared on the Group's
// inheritable Flags closure; each auth-mode leaf declares its own
// credential-specific flags and reads the Group's via cli.ApplyAncestorFlags.
//
// "locator" is TextQL's wire name for what Snowflake's own docs call
// `account` (e.g. `abc12345.us-east-1`); the CLI flag uses --locator to
// match the wire. `--database` is required; `--warehouse`/`--schema`/`--role`
// are optional per the captured UI behavior.
func newSnowflakeCreateGroup(deps Deps) *cli.Group {
	var (
		name      string
		locator   string
		database  string
		warehouse string
		schema    string
		role      string
	)
	return &cli.Group{
		Summary: "Create a Snowflake connector. Pick an auth mode.",
		Flags: func(fs *flag.FlagSet) {
			cli.DeclareString(fs, &name, "name", "", "connector name (required)")
			cli.DeclareString(fs, &locator, "locator", "", "Snowflake account locator, e.g. abc12345.us-east-1 (required)")
			cli.DeclareString(fs, &database, "database", "", "database name (required)")
			cli.DeclareString(fs, &warehouse, "warehouse", "", "default warehouse (optional)")
			cli.DeclareString(fs, &schema, "schema", "", "default schema (optional)")
			cli.DeclareString(fs, &role, "role", "", "default role (optional)")
		},
		Children: map[string]cli.Command{
			"password": &snowflakePasswordCmd{
				deps:      deps,
				name:      &name,
				locator:   &locator,
				database:  &database,
				warehouse: &warehouse,
				schema:    &schema,
				role:      &role,
			},
		},
	}
}
