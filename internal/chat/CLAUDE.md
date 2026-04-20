# internal/chat

The `ana chat` verb tree: `new`, `list`, `show`, `history`, `send` (streaming), plus `rename`, `bookmark`/`unbookmark`, `duplicate`, `delete`, and `share`. The only verb package that needs two transports: unary RPC for the CRUD surface and a streaming session for `send` (`StreamChat`).

## Files

- `chat.go` — `New`, `Deps` (Unary + Stream + UUIDFn), and both service-path constants (`ChatService` and `SharingService`; `share` reaches across to avoid standing up a sibling package for one call).
- `new.go`, `list.go`, `show.go`, `history.go`, `send.go` — the core flow. `send.go` drives `StreamSession` frame-by-frame and prints assistant text as it arrives.
- `simple.go` — the multi-subcommand file hosting `rename`, `bookmark`, `unbookmark`, `duplicate`, `delete`, and `markRead`. Collapsed into one file because each is a thin single-field mutation.
- `share.go` — `SharingService.CreateShare` (LINK_COPY verified; Slack-channel target unverified — see `api-catalog/` notes).
- `chat_test.go` — shared `fakeDeps`, `fakeStream`, `TestNew*`/`TestHelp*`, plus helper-function tests for `parseConnectorIDs` and `truncate` (both defined in `chat.go`).
- `new_test.go` / `list_test.go` / `show_test.go` / `history_test.go` / `send_test.go` / `simple_test.go` / `share_test.go` — one per source file. `send_test.go` carries the `fakeStream` frame-replay cases plus `TestFrameContentAllVariants`.
