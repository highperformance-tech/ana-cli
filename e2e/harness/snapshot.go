package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SnapshotConnector captures the current connector record keyed by id and
// registers a cleanup that restores it via UpdateConnector. Only the scalar
// fields the server lets you re-send (name, type, dialect block) are
// restored. Password fields can't be read back from the API, so mutations
// that change passwords MUST be scoped to a test-created connector
// (use CreateConnector instead) — SnapshotConnector is only safe for fields
// that round-trip through Get/Update unchanged.
func (h *H) SnapshotConnector(id int) {
	h.t.Helper()
	if h.env.dryRun {
		h.t.Logf("dryrun: snapshot connector %d", id)
		return
	}
	h.forbiddenCheck("connector", id)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var raw map[string]any
	const getPath = "/rpc/public/textql.rpc.public.connector.ConnectorService/GetConnector"
	if err := h.client.Unary(ctx, getPath, map[string]any{"connectorId": id}, &raw); err != nil {
		h.t.Fatalf("SnapshotConnector GetConnector(%d): %v", id, err)
	}
	conn, _ := raw["connector"].(map[string]any)
	if conn == nil {
		h.t.Fatalf("SnapshotConnector(%d): missing connector in response (%v)", id, raw)
	}
	// Deep-copy the snapshot so subsequent mutations via the returned map
	// don't silently corrupt the restore payload.
	snap, err := deepCopyMap(conn)
	if err != nil {
		h.t.Fatalf("SnapshotConnector(%d): deep copy: %v", id, err)
	}
	h.Register(func() {
		// Build an update body from snapshot. UpdateConnector expects
		// `connectorId` at top level and `config` alongside; re-emit the
		// full config block so any mutation in the test body is rolled back.
		cfg := map[string]any{}
		if v, ok := snap["name"]; ok {
			cfg["name"] = v
		}
		if v, ok := snap["connectorType"]; ok {
			cfg["connectorType"] = v
		}
		// Preserve whichever dialect metadata block was returned; keys like
		// postgresMetadata vs snowflakeMetadata differ per dialect.
		for k, v := range snap {
			if _, ok := v.(map[string]any); ok {
				cfg[k] = v
			}
		}
		body := map[string]any{"connectorId": id, "config": cfg}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		const updPath = "/rpc/public/textql.rpc.public.connector.ConnectorService/UpdateConnector"
		if err := h.client.Unary(ctx, updPath, body, nil); err != nil {
			h.t.Errorf("restore connector %d: %v", id, err)
			h.RecordManualRevert(
				fmt.Sprintf("connector id=%d", id),
				fmt.Sprintf("auto-restore failed: %v", err),
			)
		}
	})
}

// SnapshotChatName captures the chat's current summary and defers a rename
// back to that value. Safe to use on chats the test did NOT create, as long
// as only the `summary` field is mutated.
func (h *H) SnapshotChatName(id string) {
	h.t.Helper()
	if h.env.dryRun {
		h.t.Logf("dryrun: snapshot chat name %s", id)
		return
	}
	h.forbiddenCheck("chat", id)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var raw map[string]any
	const getPath = "/rpc/public/textql.rpc.public.chat.ChatService/GetChat"
	if err := h.client.Unary(ctx, getPath, map[string]any{"chatId": id}, &raw); err != nil {
		h.t.Fatalf("SnapshotChatName GetChat(%s): %v", id, err)
	}
	summary := ""
	if chat, ok := raw["chat"].(map[string]any); ok {
		if s, ok := chat["summary"].(string); ok {
			summary = s
		}
	}
	h.Register(func() {
		body := map[string]any{"chatId": id, "summary": summary}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		const updPath = "/rpc/public/textql.rpc.public.chat.ChatService/UpdateChat"
		if err := h.client.Unary(ctx, updPath, body, nil); err != nil {
			h.t.Errorf("restore chat %s summary: %v", id, err)
			h.RecordManualRevert(
				fmt.Sprintf("chat id=%s summary", id),
				fmt.Sprintf("auto-restore failed: %v", err),
			)
		}
	})
}

// deepCopyMap clones a map[string]any through a JSON round-trip. Sufficient
// for server responses which are always JSON-shaped.
func deepCopyMap(m map[string]any) (map[string]any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
