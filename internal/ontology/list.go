package ontology

import (
	"context"
	"fmt"
	"io"

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
	var raw map[string]any
	if err := c.deps.Unary(ctx, ontologyServicePath+"/GetOntologies", struct{}{}, &raw); err != nil {
		return fmt.Errorf("ontology list: %w", err)
	}
	var typed listResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *listResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tNAME")
		for _, o := range t.Ontologies {
			fmt.Fprintf(tw, "%d\t%s\n", o.ID, o.Name)
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("ontology list: %w", err)
	}
	return nil
}
