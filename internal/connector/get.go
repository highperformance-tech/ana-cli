package connector

import (
	"context"
	"fmt"

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
	fs := cli.NewFlagSet("connector get")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.UsageErrf("connector get: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector get", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnector", getReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector get: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	conn, _ := raw["connector"].(map[string]any)
	if conn == nil {
		// Fall back to raw dump rather than render an empty table; still use
		// cli.WriteJSON so callers can diagnose unexpected shapes.
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	return cli.RenderTwoCol(stdio.Stdout, conn)
}
