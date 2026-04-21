package connector

import (
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// newCreateGroup wires the `ana connector create` subtree. Each child is a
// Group for a specific dialect; each dialect's children are leaves for each
// supported auth mode. The two-level shape scales: adding a dialect is a new
// file, adding an auth mode is a sibling leaf under the dialect Group — no
// N×M conditional matrix on a flat `--type`/`--auth` flag pair.
//
// Breaking change from v0.x: `ana connector create --type postgres …` became
// `ana connector create postgres password …`.
func newCreateGroup(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Create a new connector. Pick a dialect, then an auth mode.",
		Children: map[string]cli.Command{
			"postgres":  newPostgresCreateGroup(deps),
			"snowflake": newSnowflakeCreateGroup(deps),
		},
	}
}

// resolveSecret resolves a required secret (password, OAuth client secret,
// PAT, …) from either --<name>-stdin (reads one line from r via
// cli.ReadPassword, preserving every byte except the trailing line
// terminator) or --<name>. If both are set, --<name>-stdin wins (it's the
// more secure channel). Neither set → usage error. Preserving surrounding
// whitespace is intentional: a secret may legitimately start or end with
// spaces/tabs, and silently trimming would cause hard-to-diagnose auth
// failures.
//
// flagName is the flag base (e.g. "password", "oauth-client-secret") used
// only in error messages so they read as `--<name>-stdin set but stdin was
// empty`.
//
// Lives in create.go rather than a per-dialect file because update.go also
// reuses it when a secret flag is supplied on an edit.
func resolveSecret(flagName, secretVal string, stdinFlag bool, r io.Reader) (string, error) {
	if stdinFlag {
		pass, err := cli.ReadPassword(r)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", flagName, err)
		}
		if pass == "" {
			return "", cli.UsageErrf("--%s-stdin set but stdin was empty", flagName)
		}
		return pass, nil
	}
	if secretVal == "" {
		return "", cli.UsageErrf("--%s or --%s-stdin is required", flagName, flagName)
	}
	return secretVal, nil
}
