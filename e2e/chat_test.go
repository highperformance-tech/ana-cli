package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

// chatIDByTitle walks `chat list --json` and returns the id of the first
// row whose summary equals the given title. Empty means not found.
func chatIDByTitle(t *testing.T, h *harness.H, title string) string {
	t.Helper()
	raw, err := h.RunJSON("chat", "list")
	if err != nil {
		t.Fatalf("chat list --json: %v", err)
	}
	arr, _ := raw["chats"].([]any)
	for _, item := range arr {
		entry, _ := item.(map[string]any)
		if s, _ := entry["summary"].(string); s == title {
			id, _ := entry["id"].(string)
			return id
		}
	}
	return ""
}

// TestChatNewShowDelete covers the Tier-1 create -> show -> delete chain.
// Deferred DeleteChat is registered by CreateChat; test body only exercises
// the read-side to confirm the chat actually lives server-side.
func TestChatNewShowDelete(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("chat", connSpecFromEnv())
	chatID := h.CreateChat("chat", []int{conn})
	if chatID == "" && !h.DryRun() {
		t.Fatalf("CreateChat returned empty id")
	}
	out, stderr, err := h.Run("chat", "show", chatID)
	if err != nil {
		t.Fatalf("chat show: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	if out == "" {
		t.Errorf("chat show produced no output (stderr=%s)", stderr)
	}
}

// TestChatRename exercises UpdateChat. Because the chat was created by the
// test, this collapses into Tier 1 — deleting the parent invalidates the
// rename anyway, no snapshot needed.
func TestChatRename(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("rename", connSpecFromEnv())
	chatID := h.CreateChat("rename", []int{conn})
	newTitle := h.ResourceName("rename-new")
	if _, stderr, err := h.Run("chat", "rename", chatID, newTitle); err != nil {
		t.Fatalf("chat rename: %v\nstderr: %s", err, stderr)
	}
	// `chat list` is the cheapest way to confirm the summary updated.
	out, stderr, err := h.Run("chat", "list")
	if err != nil {
		t.Fatalf("chat list: %v\nstderr: %s", err, stderr)
	}
	if !h.DryRun() && !strings.Contains(out, newTitle) {
		t.Errorf("rename did not surface in chat list: %s (stderr=%s)", out, stderr)
	}
}

// TestChatShareChain covers the new -> share -> delete chain. The plan
// specifies that deleting the parent chat invalidates the share link
// server-side, so no ledger entry is needed.
func TestChatShareChain(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("share", connSpecFromEnv())
	chatID := h.CreateChat("share", []int{conn})
	out, stderr, err := h.Run("chat", "share", chatID)
	if err != nil {
		t.Fatalf("chat share: %v\nstderr: %s", err, stderr)
	}
	if h.DryRun() {
		return
	}
	trimmed := strings.TrimSpace(out)
	// Share output is either a URL or a token; either way, non-empty.
	if trimmed == "" {
		t.Errorf("share produced empty output (stderr=%s)", stderr)
	}
}

// TestChatSend streams a short message through `chat send` and asserts the
// stream closed cleanly. The message is intentionally tiny so the test
// stays fast even when the backend is cold.
func TestChatSend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping streaming test in -short mode")
	}
	h := harness.Begin(t)
	conn := h.CreateConnector("send", connSpecFromEnv())
	chatID := h.CreateChat("send", []int{conn})
	if chatID == "" && h.DryRun() {
		return
	}
	if _, stderr, err := h.Run("chat", "send", chatID, "hello, this is an e2e smoke test"); err != nil {
		t.Fatalf("chat send: %v\nstderr: %s", err, stderr)
	}
}

// TestChatDuplicateDelete covers the duplicate verb. The duplicate's id is
// captured via --json and registered for deferred delete so we don't leak a
// second chat on pass.
func TestChatDuplicateDelete(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("dup", connSpecFromEnv())
	parentID := h.CreateChat("dup", []int{conn})
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("chat", "duplicate", parentID)
	if err != nil {
		t.Fatalf("chat duplicate: %v", err)
	}
	child, _ := raw["chat"].(map[string]any)
	if child == nil {
		t.Fatalf("chat duplicate: response missing chat object: %v", raw)
	}
	dupID, _ := child["id"].(string)
	if dupID == "" {
		t.Fatalf("chat duplicate: empty id in response: %v", raw)
	}
	h.Register(func() {
		if _, _, err := h.Run("chat", "delete", dupID); err != nil {
			h.RecordManualRevert(
				fmt.Sprintf("duplicated chat id=%s", dupID),
				fmt.Sprintf("auto-delete failed: %v", err),
			)
		}
	})
}

// TestChatListJSON asserts --json emits a `chats` array.
func TestChatListJSON(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	raw, err := h.RunJSON("chat", "list")
	if err != nil {
		t.Fatalf("chat list --json: %v", err)
	}
	if _, ok := raw["chats"]; !ok {
		t.Errorf("--json response missing `chats` key: %v", raw)
	}
}

// TestChatShowJSON confirms --json returns a `chat` object whose id matches.
func TestChatShowJSON(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("show-json", connSpecFromEnv())
	chatID := h.CreateChat("show-json", []int{conn})
	if chatID == "" {
		if h.DryRun() {
			return
		}
		t.Fatalf("CreateChat returned empty id")
	}
	raw, err := h.RunJSON("chat", "show", chatID)
	if err != nil {
		t.Fatalf("chat show --json: %v", err)
	}
	chat, ok := raw["chat"].(map[string]any)
	if !ok {
		t.Fatalf("--json response missing `chat` object: %v", raw)
	}
	if got, _ := chat["id"].(string); got != chatID {
		t.Errorf("chat.id = %q, want %q", got, chatID)
	}
}

// TestChatNewCLI drives `chat new --connector <id> --title <name>` through
// the CLI. The command emits the new chat id on stdout; we register
// `chat delete` for cleanup since the harness's parent-nesting path was
// bypassed.
func TestChatNewCLI(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("new-cli", connSpecFromEnv())
	if h.DryRun() {
		_, _, _ = h.Run("chat", "new", "--connector", fmt.Sprint(conn),
			"--title", h.ResourceName("new-cli"))
		return
	}
	title := h.ResourceName("new-cli")
	stdout, stderr, err := h.Run("chat", "new",
		"--connector", fmt.Sprint(conn),
		"--title", title)
	if err != nil {
		t.Fatalf("chat new: %v\nstderr: %s", err, stderr)
	}
	chatID := strings.TrimSpace(stdout)
	if chatID == "" {
		t.Fatalf("chat new: empty id on stdout")
	}
	h.Register(func() {
		if _, delStderr, err := h.Run("chat", "delete", chatID); err != nil {
			h.RecordManualRevert(
				fmt.Sprintf("chat id=%s", chatID),
				fmt.Sprintf("CLI-path delete failed: %v stderr=%s", err, delStderr),
			)
		}
	})
	// Round-trip: the new chat should show up in `chat list` under the title.
	if id := chatIDByTitle(t, h, title); id != chatID {
		t.Errorf("chat list lookup of %q = %q, want %q", title, id, chatID)
	}
}

// TestChatHistory runs `chat history <id>` against a freshly-created chat.
// A new chat with no messages yields an empty table; the smoke is that the
// endpoint returns without error.
func TestChatHistory(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("hist", connSpecFromEnv())
	chatID := h.CreateChat("hist", []int{conn})
	if chatID == "" {
		if h.DryRun() {
			return
		}
		t.Fatalf("CreateChat returned empty id")
	}
	if _, stderr, err := h.Run("chat", "history", chatID); err != nil {
		t.Fatalf("chat history: %v\nstderr: %s", err, stderr)
	}
}

// TestChatBookmarkUnbookmark exercises the bookmark + unbookmark chain. Both
// emit `ok` on success; deletion of the parent chat naturally clears any
// bookmark state, so no separate ledger entry is needed.
func TestChatBookmarkUnbookmark(t *testing.T) {
	h := harness.Begin(t)
	conn := h.CreateConnector("bm", connSpecFromEnv())
	chatID := h.CreateChat("bm", []int{conn})
	if chatID == "" {
		if h.DryRun() {
			return
		}
		t.Fatalf("CreateChat returned empty id")
	}
	bmOut, stderr, err := h.Run("chat", "bookmark", chatID)
	if err != nil {
		t.Fatalf("chat bookmark: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(bmOut, "ok") {
		t.Errorf("chat bookmark: expected ok on stdout, got %q", bmOut)
	}
	unbmOut, stderr, err := h.Run("chat", "unbookmark", chatID)
	if err != nil {
		t.Fatalf("chat unbookmark: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(unbmOut, "ok") {
		t.Errorf("chat unbookmark: expected ok on stdout, got %q", unbmOut)
	}
}

// TestChatShowNotFound covers the error-path: `chat show <missing>` must
// exit non-zero.
func TestChatShowNotFound(t *testing.T) {
	h := harness.Begin(t)
	if h.DryRun() {
		return
	}
	_, _, err := h.Run("chat", "show", "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatalf("chat show <missing>: expected error, got nil")
	}
}
