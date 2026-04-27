package connector

import (
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// tablesCmd implements `ana connector tables <id>` — ListConnectorTables.
// Renders a SCHEMA/NAME two-column table; `--json` dumps the raw response.
type tablesCmd struct{ deps Deps }

func (c *tablesCmd) Help() string {
	return "tables   List tables exposed by a connector.\n" +
		"Usage: ana connector tables <id>"
}

type tablesReq struct {
	ConnectorID int `json:"connectorId"`
}

type tablesResp struct {
	Tables []struct {
		TableSchema string `json:"tableSchema"`
		TableName   string `json:"tableName"`
	} `json:"tables"`
}

func (c *tablesCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 1 {
		return cli.UsageErrf("connector tables: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector tables", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/ListConnectorTables", tablesReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector tables: %w", err)
	}
	var typed tablesResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *tablesResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "SCHEMA\tNAME")
		for _, row := range t.Tables {
			fmt.Fprintf(tw, "%s\t%s\n", row.TableSchema, row.TableName)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("connector tables: %w", err)
	}
	return nil
}
