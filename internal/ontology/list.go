package ontology

import (
	"context"
	"fmt"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// listCmd implements `ana ontology list` — GetOntologies with `{}`. Table
// columns: ID, NAME. Catalog shows GetOntologies is the primary readonly
// endpoint; GetOntologiesSummary is a second option present in the catalog
// but not used here.
type listCmd struct{ deps Deps }

func (c *listCmd) Help() string {
	return "list   List ontologies (ID/NAME table, --json for raw).\n" +
		"Usage: ana ontology list"
}

// listResp narrows the fields we render. The catalog has many more fields on
// each ontology (description, connectorId, objects, attributes, ...); the
// decoder drops them. ID is an integer on the wire.
type listResp struct {
	Ontologies []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"ontologies"`
}

// Run issues GetOntologies and prints either a table or the raw payload.
func (c *listCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := cli.NewFlagSet("ontology list")
	if err := cli.ParseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, ontologyServicePath+"/GetOntologies", struct{}{}, &raw); err != nil {
		return fmt.Errorf("ontology list: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return cli.WriteJSON(stdio.Stdout, raw)
	}
	var typed listResp
	if err := cli.Remarshal(raw, &typed); err != nil {
		return fmt.Errorf("ontology list: decode response: %w", err)
	}
	tw := cli.NewTableWriter(stdio.Stdout)
	fmt.Fprintln(tw, "ID\tNAME")
	for _, o := range typed.Ontologies {
		fmt.Fprintf(tw, "%d\t%s\n", o.ID, o.Name)
	}
	return tw.Flush()
}
