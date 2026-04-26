package connector

import (
	"context"
	"fmt"
	"io"

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
	if len(args) != 0 {
		return cli.UsageErrf("connector list: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnectors", struct{}{}, &raw); err != nil {
		return fmt.Errorf("connector list: %w", err)
	}
	var typed listResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME\tTYPE")
		for _, k := range t.Connectors {
			fmt.Fprintf(tw, "%d\t%s\t%s\n", k.ID, k.Name, k.ConnectorType)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("connector list: %w", err)
	}
	return nil
}
