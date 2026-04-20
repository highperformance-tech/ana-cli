package connector

import (
	"context"
	"fmt"
	"io"

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

// getResp keeps Connector as a free-form map so the renderer can dispatch to
// RenderTwoCol without having to enumerate every server-added field.
type getResp struct {
	Connector map[string]any `json:"connector"`
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
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetConnector", getReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector get: %w", err)
	}
	var typed getResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *getResp) error {
		if t.Connector == nil {
			// Fall back to raw dump rather than render an empty table; still
			// use cli.WriteJSON so callers can diagnose unexpected shapes.
			return cli.WriteJSON(w, raw)
		}
		return cli.RenderTwoCol(w, t.Connector)
	}); err != nil {
		return fmt.Errorf("connector get: %w", err)
	}
	return nil
}
