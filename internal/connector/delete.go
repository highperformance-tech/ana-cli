package connector

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// deleteCmd implements `ana connector delete <id>` — DeleteConnector returns
// `{success: true}`, which we don't surface; the user gets a confirmation line.
type deleteCmd struct{ deps Deps }

func (c *deleteCmd) Help() string {
	return "delete   Delete a connector by id.\n" +
		"Usage: ana connector delete <id>"
}

type deleteReq struct {
	ConnectorID int `json:"connectorId"`
}

func (c *deleteCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if len(args) != 1 {
		return cli.UsageErrf("connector delete: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector delete", args)
	if err != nil {
		return err
	}
	if err := c.deps.Unary(ctx, servicePath+"/DeleteConnector", deleteReq{ConnectorID: id}, nil); err != nil {
		return fmt.Errorf("connector delete: %w", err)
	}
	fmt.Fprintf(stdio.Stdout, "deleted %d\n", id)
	return nil
}
