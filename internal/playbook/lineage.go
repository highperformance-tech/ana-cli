package playbook

import (
	"cmp"
	"context"
	"fmt"
	"io"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// lineageCmd implements `ana playbook lineage <id>` — GetPlaybookLineage with
// `{playbookId: "..."}`. Prints edges (src -> dst with an optional type
// column) as a table; --json prints the raw payload.
//
// Catalog deviation: the sole captured sample has an empty response body
// (`{}`), so the edge shape is inferred. We accept either `edges`, `nodes`,
// or `lineage` as the top-level key and look for `from`/`to` or
// `source`/`target` fields on each edge. Any mismatch prints "(no lineage
// edges)" — the user can fall back to --json to see the actual shape.
type lineageCmd struct{ deps Deps }

func (c *lineageCmd) Help() string {
	return "lineage   Print a playbook's lineage edges (or --json for raw).\n" +
		"Usage: ana playbook lineage <id>"
}

type lineageReq struct {
	PlaybookID string `json:"playbookId"`
}

// lineageResp tolerates several plausible shapes since the capture sample is
// empty. We try `edges` first, then `lineage`, then `nodes`. Each entry may
// expose from/to or source/target pairs.
type lineageEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type lineageResp struct {
	Edges   []lineageEdge `json:"edges"`
	Lineage []lineageEdge `json:"lineage"`
	Nodes   []lineageEdge `json:"nodes"`
}

func (c *lineageCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	if err := cli.RequireMaxPositionals("playbook lineage", 1, args); err != nil {
		return err
	}
	id, err := cli.RequireStringID("playbook lineage", args)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, playbookServicePath+"/GetPlaybookLineage", lineageReq{PlaybookID: id}, &raw); err != nil {
		return fmt.Errorf("playbook lineage: %w", err)
	}
	var typed lineageResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *lineageResp) error {
		edges := t.Edges
		if len(edges) == 0 {
			edges = t.Lineage
		}
		if len(edges) == 0 {
			edges = t.Nodes
		}
		if len(edges) == 0 {
			fmt.Fprintln(w, "(no lineage edges)")
			return nil
		}
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "FROM\tTO\tTYPE")
		for _, e := range edges {
			from := cli.DashIfEmpty(cmp.Or(e.From, e.Source))
			to := cli.DashIfEmpty(cmp.Or(e.To, e.Target))
			fmt.Fprintf(tw, "%s\t%s\t%s\n", from, to, cli.DashIfEmpty(e.Type))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("playbook lineage: %w", err)
	}
	return nil
}
