// Package feed provides the `ana feed` verb tree: show (posts) and stats.
// Like the other verb packages it avoids importing internal/transport and
// internal/config — callers inject a narrow Deps struct that adapts a real
// transport client to a single Unary function field.
package feed

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

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

// newFlagSet returns a FlagSet with ContinueOnError + silenced output.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags delegates to cli.ParseFlags so positional args can be
// interleaved with flags without silently dropping trailing flags.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return cli.ParseFlags(fs, args)
}

// writeJSON indents a value to w with the 2-space convention.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// remarshal round-trips src through JSON into dst.
func remarshal(src, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
