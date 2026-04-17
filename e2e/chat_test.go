package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/highperformance-tech/ana-cli/e2e/harness"
)

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
