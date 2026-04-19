package connector

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// examplesCmd implements `ana connector examples <id>` — GetExampleQueries.
//
// CATALOG NOTE: the request body wraps the connector id in a
// `connectorContexts` array (see the captured sample), not a bare
// `{connectorId}`. We follow the catalog.
type examplesCmd struct{ deps Deps }

func (c *examplesCmd) Help() string {
	return "examples   Show example queries suggested for a connector.\n" +
		"Usage: ana connector examples <id>"
}

type examplesContext struct {
	ConnectorID int `json:"connectorId"`
}

type examplesReq struct {
	ConnectorContexts []examplesContext `json:"connectorContexts"`
}

type examplesResp struct {
	Examples []struct {
		Label    string `json:"label"`
		Message  string `json:"message"`
		Category string `json:"category"`
	} `json:"examples"`
}

func (c *examplesCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector examples")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.UsageErrf("connector examples: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector examples", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	req := examplesReq{ConnectorContexts: []examplesContext{{ConnectorID: id}}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetExampleQueries", req, &raw); err != nil {
		return fmt.Errorf("connector examples: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed examplesResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("connector examples: decode response: %w", err)
	}
	for _, ex := range typed.Examples {
		fmt.Fprintf(stdio.Stdout, "- [%s] %s: %s\n", ex.Category, ex.Label, ex.Message)
	}
	return nil
}
