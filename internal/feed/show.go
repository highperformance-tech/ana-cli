package feed

import (
	"context"
	"fmt"
	"io"

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
	if len(args) != 0 {
		return cli.UsageErrf("feed show: unexpected positional arguments: %v", args)
	}
	var raw map[string]any
	if err := c.deps.Unary(ctx, feedServicePath+"/GetFeed", struct{}{}, &raw); err != nil {
		return fmt.Errorf("feed show: %w", err)
	}
	var typed showResp
	if err := cli.RenderOutput(stdio.Stdout, raw, cli.GlobalFrom(ctx).JSON, &typed, func(w io.Writer, t *showResp) error {
		tw := cli.NewTableWriter(w)
		fmt.Fprintln(tw, "ID\tTITLE\tAGENT\tUPVOTES\tCREATED")
		for _, p := range t.Posts {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
				p.ID,
				cli.DashIfEmpty(p.Title),
				cli.DashIfEmpty(p.CreatorAgentName),
				p.UpvoteCount,
				cli.DashIfEmpty(p.CreatedAt))
		}
		return tw.Flush()
	}); err != nil {
		return fmt.Errorf("feed show: %w", err)
	}
	return nil
}
