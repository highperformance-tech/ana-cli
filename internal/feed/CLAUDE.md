# internal/feed

The `ana feed` verb tree: `show` (posts) and `stats`. Streaming variant not captured. Dispatch-only around `Deps.Unary`.

## Files

- `feed.go` — `New`, `Deps`, service path prefix (`FeedService`).
- `show.go` — `GetFeed`.
- `stats.go` — `GetFeedStats`.
- `feed_test.go` — fake `Unary` covers each subcommand.
