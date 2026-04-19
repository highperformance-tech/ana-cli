// Package feed provides the `ana feed` verb tree: show (posts) and stats.
// Like the other verb packages it avoids importing internal/transport and
// internal/config — callers inject a narrow Deps struct that adapts a real
// transport client to a single Unary function field.
package feed

import (
	"context"

	"github.com/highperformance-tech/ana-cli/internal/cli"
)

// feedServicePath is the Connect-RPC prefix every FeedService endpoint uses.
const feedServicePath = "/rpc/public/textql.rpc.public.feed.FeedService"

// Deps is the injection boundary for the feed package.
type Deps struct {
	Unary func(ctx context.Context, path string, req, resp any) error
}

// New returns the `feed` verb group.
func New(deps Deps) *cli.Group {
	return &cli.Group{
		Summary: "Inspect the org feed: show, stats.",
		Children: map[string]cli.Command{
			"show":  &showCmd{deps: deps},
			"stats": &statsCmd{deps: deps},
		},
	}
}
