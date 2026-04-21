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

// resolvePassword resolves the password from either --password-stdin (reads
// one line from r via cli.ReadPassword, preserving every byte except the
// trailing line terminator) or --password. If both are set, --password-stdin
// wins (it's the more secure channel). Neither set → usage error. Preserving
// surrounding whitespace is intentional: a password may legitimately start or
// end with spaces/tabs, and silently trimming would cause hard-to-diagnose
// auth failures.
//
// Lives in create.go rather than a per-dialect file because update.go also
// reuses it when --password{,-stdin} is supplied on an edit.
func resolvePassword(passFlag string, stdinFlag bool, r io.Reader) (string, error) {
	if stdinFlag {
		pass, err := cli.ReadPassword(r)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		if pass == "" {
			return "", cli.UsageErrf("--password-stdin set but stdin was empty")
		}
		return pass, nil
	}
	if passFlag == "" {
		return "", cli.UsageErrf("--password or --password-stdin is required")
	}
	return passFlag, nil
}
