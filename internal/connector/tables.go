package connector

import (
	"context"
	"fmt"

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
	fs := cli.NewFlagSet("connector tables")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.UsageErrf("connector tables: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector tables", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/ListConnectorTables", tablesReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector tables: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed tablesResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("connector tables: decode response: %w", err)
	}
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "SCHEMA\tNAME")
	for _, t := range typed.Tables {
		fmt.Fprintf(tw, "%s\t%s\n", t.TableSchema, t.TableName)
	}
	return tw.Flush()
}
