package feed

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// showCmd implements `ana feed show` — GetFeed with `{}`. Catalog shows the
// response envelope is `{ "posts": [ ... ] }`; each post carries id, title,
// creatorAgentName, upvoteCount, createdAt and more.
type showCmd struct{ deps Deps }

func (c *showCmd) Help() string {
	return "show   Show recent feed posts (ID/TITLE/AGENT/UPVOTES/CREATED table, --json for raw).\n" +
		"Usage: ana feed show"
}

// showResp narrows the fields we render.
type showResp struct {
	Posts []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		CreatorAgentName string `json:"creatorAgentName"`
		UpvoteCount      int64  `json:"upvoteCount"`
		CreatedAt        string `json:"createdAt"`
	} `json:"posts"`
}

func (c *showCmd) Run(ctx context.Context, args []string, stdio cli.IO) error {
	fs := newFlagSet("feed show")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, feedServicePath+"/GetFeed", struct{}{}, &raw); err != nil {
		return fmt.Errorf("feed show: %w", err)
	}
	if cli.GlobalFrom(ctx).JSON {
		return writeJSON(stdio.Stdout, raw)
	}
	var typed showResp
	if err := remarshal(raw, &typed); err != nil {
		return fmt.Errorf("feed show: decode response: %w", err)
	}
	tw := tabwriter.NewWriter(stdio.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tAGENT\tUPVOTES\tCREATED")
	for _, p := range typed.Posts {
		agent := p.CreatorAgentName
		if agent == "" {
			agent = "-"
		}
		title := p.Title
		if title == "" {
			title = "-"
		}
		created := p.CreatedAt
		if created == "" {
			created = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", p.ID, title, agent, p.UpvoteCount, created)
	}
	return tw.Flush()
}
