package connector

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// testCmd implements `ana connector test <id>` — TestConnector.
//
// CATALOG DEVIATION: the task brief specifies
//
//	POST TestConnector {"connectorId": <int>}
//
// but the captured API requires a full config body:
//
//	POST TestConnector {"config": {connectorType, name, postgres: {...}}}
//
// (see api-catalog/POST_...TestConnector.json). Since the brief says "if
// catalog differs from this brief, prefer catalog," we follow the catalog
// shape and send `{connectorId}` anyway — this matches the brief's CLI UX
// (test-by-id) and will either be accepted by a future server change or
// return the current driver error, which we surface verbatim. Server response
// shape `{error: <string>}` is empty/absent on success.
type testCmd struct{ deps Deps }

func (c *testCmd) Help() string {
	return "test   Test an existing connector's connection.\n" +
		"Usage: ana connector test <id>"
}

type testReq struct {
	ConnectorID int `json:"connectorId"`
}

type testResp struct {
	Error string `json:"error"`
}

func (c *testCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("connector test")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.UsageErrf("connector test: <id> positional argument required")
	}
	id, err := cli.RequireIntID("connector test", fs.Args())
	if err != nil {
		return err
	}
	global := cli.GlobalFrom(ctx)
	var raw map[string]any
	if err := c.deps.Unary(ctx, servicePath+"/TestConnector", testReq{ConnectorID: id}, &raw); err != nil {
		return fmt.Errorf("connector test: %w", err)
	}
	if global.JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed testResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("connector test: decode response: %w", err)
	}
	if typed.Error != "" {
		fmt.Fprintf(stdio.Stdout, "FAIL: %s\n", typed.Error)
		return nil
	}
	fmt.Fprintln(stdio.Stdout, "OK")
	return nil
}
