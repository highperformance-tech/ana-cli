package connector

import (
	"context"
	"fmt"
	"io"

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
	if len(args) != 1 {
		return cli.UsageErrf("connector examples: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector examples", args)
	if err != nil {
		return err
	}
	req := examplesReq{ConnectorContexts: []examplesContext{{ConnectorID: id}}}
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/GetExampleQueries", req, &raw); err != nil {
		return fmt.Errorf("connector examples: %w", err)
	}
	var typed examplesResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *examplesResp) error {
		for _, ex := range t.Examples {
			if _, err := fmt.Fprintf(w, "- [%s] %s: %s\n", ex.Category, ex.Label, ex.Message); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("connector examples: %w", err)
	}
	return nil
}
