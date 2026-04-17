package connector

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd implements `ana connector list` — GetConnectors with an empty body.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List connectors (table by default, --json for raw JSON).\n" +
		"Usage: ana connector list"
}

// listResp is the narrow shape we render as a table. Extra fields in the API
// response (memberId, createdAt, <dialect>Metadata, authStrategy, etc.) are
// silently dropped by the decoder.
type listResp struct {
	Connectors []struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		ConnectorType string `json:"connectorType"`
	} `json:"connectors"`
}

// Run issues GetConnectors then either dumps raw JSON or prints a fixed-width
// ID/NAME/TYPE table.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("connector list")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnectors", struct{}{}, &raw); err != nil {
		return fmt.Errorf("connector list: %w", err)
	}
	if global.JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed listResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("connector list: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tTYPE")
	for _, k := range typed.Connectors {
		fmt.Fprintf(tw, "%d\t%s\t%s\n", k.ID, k.Name, k.ConnectorType)
	}
	return tw.Flush()
}
