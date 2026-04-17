package connector

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// getCmd implements `ana connector get <id>` — GetConnector with a single
// integer connectorId. The default render is a YAML-ish two-column view of the
// top-level fields plus the dialect-specific metadata block.
type getCmd struct{ deps Deps }

func (c *getCmd) Help() string {
	return "get   Show a connector's details.\n" +
		"Usage: ana connector get <id>"
}

// getReq keeps wire-level field naming explicit (camelCase).
type getReq struct {
	ConnectorID int `json:"connectorId"`
}

func (c *getCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("connector get")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErrf("connector get: <id> positional argument required")
	}
	id, err := atoiID("connector get", fs.Arg(0))
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnector", getReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector get: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	conn, _ := raw["connector"].(map[string]any)
	if conn == nil {
		// Fall back to raw dump rather than render an empty table; still use
		// writeJSON so callers can diagnose unexpected shapes.
		return writeJSON(stdio.Stdout, raw)
	}
	return renderTwoCol(stdio.Stdout, conn)
}

// renderTwoCol prints top-level scalar fields then any nested map fields
// (e.g. postgresMetadata) as an indented sub-block. Keys are sorted so the
// output is stable across runs for snapshot-style tests.
func renderTwoCol(w interface {
	Write(p []byte) (int, error)
}, m map[string]any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	scalarKeys := make([]string, 0, len(m))
	nestedKeys := make([]string, 0)
	for k, v := range m {
		if _, ok := v.(map[string]any); ok {
			nestedKeys = append(nestedKeys, k)
			continue
		}
		scalarKeys = append(scalarKeys, k)
	}
	sort.Strings(scalarKeys)
	sort.Strings(nestedKeys)
	for _, k := range scalarKeys {
		fmt.Fprintf(tw, "%s:\t%v\n", k, m[k])
	}
	for _, k := range nestedKeys {
		fmt.Fprintf(tw, "%s:\t\n", k)
		sub := m[k].(map[string]any)
		subKeys := make([]string, 0, len(sub))
		for sk := range sub {
			subKeys = append(subKeys, sk)
		}
		sort.Strings(subKeys)
		for _, sk := range subKeys {
			fmt.Fprintf(tw, "  %s:\t%v\n", sk, sub[sk])
		}
	}
	return tw.Flush()
}
