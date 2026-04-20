# internal/feed

The `ana feed` verb tree: `show` (posts) and `stats`. Streaming variant not captured. Dispatch-only around `Deps.Unary`.

## Files

- `feed.go` — `New`, `Deps`, service path prefix (`FeedService`).
- `show.go` — `GetFeed`.
- `stats.go` — `GetFeedStats`.
- `feed_test.go` — shared `fakeDeps` + `TestNew*`/`TestHelp*`.
- `show_test.go` / `stats_test.go` — one per source file; `stats_test.go` also covers the `joinOrDash` helper defined in `stats.go`.
