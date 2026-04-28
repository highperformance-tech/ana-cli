# internal/chat

The `ana chat` verb tree: `new`, `list`, `show`, `history`, `send` (streaming), plus `rename`, `bookmark`/`unbookmark`, `duplicate`, `delete`, and `share`. The only verb package that needs two transports: unary RPC for the CRUD surface and a streaming session for `send` (`StreamChat`).

## Files

- `chat.go` — `New`, `Deps` (Unary + Stream + UUIDFn), the `ChatService` + `SharingService` path constants (share crosses services to avoid a sibling package for one call), and the rune-safe `truncate` helper.
- `new.go`, `list.go`, `show.go`, `history.go`, `send.go` — core flow; `send.go` drives `StreamSession` frame-by-frame.
- `simple.go` — single-field mutations collapsed together: `rename`, `bookmark`, `unbookmark`, `duplicate`, `delete`, `markRead`.
- `share.go` — `SharingService.CreateShare` (LINK_COPY verified; Slack target unverified).
- `*_test.go` — one per source. Shared fakes (`fakeDeps`, `fakeStream`) live in `chat_test.go`.
