package ontology

import (
	"context"
	"fmt"
	"strconv"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// getCmd implements `ana ontology get <id>` — GetOntologyById with
// `{ontologyId: <int>}`. Catalog confirms the request takes an integer id
// (not a string): per `inferredRequestSchema` the field is `"integer"`.
type getCmd struct{ deps Deps }

func (c *getCmd) Help() string {
	return "get   Show an ontology's main fields (--json for raw).\n" +
		"Usage: ana ontology get <id>"
}

// getReq is the exact wire shape — catalog confirms a single integer
// `ontologyId`.
type getReq struct {
	OntologyID int64 `json:"ontologyId"`
}

// getResp is the compact typed projection.
type getResp struct {
	Ontology struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		ConnectorID int64  `json:"connectorId"`
	} `json:"ontology"`
}

func (c *getCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("ontology get")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	raw, err := cli.RequireStringID("ontology get", fs.Args())
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return cli.UsageErrf("ontology get: <id> must be an integer: %v", err)
	}
	var rawResp map[string]any
	if err := c.deps.Unary(ctx, ontologyServicePath+"/GetOntologyById", getReq{OntologyID: id}, &rawResp); err != nil {
		return fmt.Errorf("ontology get: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, rawResp)
	}
	var typed getResp
	if err := cli.Remarshal(rawResp, &typed); err != nil {
		return fmt.Errorf("ontology get: decode response: %w", err)
	}
	// A missing `ontology` envelope falls through to --json so the user sees
	// the response shape rather than a block of empty fields.
	if typed.Ontology.ID == 0 && typed.Ontology.Name == "" {
		return cli.WriteJSON(stdio.Stdout, rawResp)
	}
	o := typed.Ontology
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintf(tw, "id\t%d\n", o.ID)
	fmt.Fprintf(tw, "name\t%s\n", o.Name)
	fmt.Fprintf(tw, "description\t%s\n", o.Description)
	fmt.Fprintf(tw, "connectorId\t%d\n", o.ConnectorID)
	return tw.Flush()
}
