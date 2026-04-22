package harness

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Tier-1 resource helpers. Each Create*() issues the RPC directly via the
// transport client, registers a cleanup that deletes the resource (LIFO), and
// returns the server-assigned id. Names are forced through h.ResourceName so
// sweep.go can find leftovers.

// ConnSpec is the minimum postgres-dialect spec the e2e suite exercises.
// Keep fields narrow so the test data dir does not turn into a config editor.
type ConnSpec struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSL      bool
}

// CreateConnector posts CreateConnector and defers DeleteConnector.
func (h *H) CreateConnector(suffix string, spec ConnSpec) int {
	h.t.Helper()
	name := h.ResourceName(suffix)
	if h.env.dryRun {
		h.t.Logf("dryrun: create connector %q", name)
		return 0
	}
	req := map[string]any{
		"config": map[string]any{
			"connectorType": "POSTGRES",
			"name":          name,
			"postgres": map[string]any{
				"host":     spec.Host,
				"port":     spec.Port,
				"user":     spec.User,
				"password": spec.Password,
				"database": spec.Database,
				"sslMode":  spec.SSL,
			},
		},
	}
	var resp struct {
		ConnectorID int `json:"connectorId"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.connector.ConnectorService/CreateConnector"
	if err := h.client.Unary(ctx, path, req, &resp); err != nil {
		h.t.Fatalf("CreateConnector: %v", err)
	}
	id := resp.ConnectorID
	h.Register(func() { h.deleteConnector(id) })
	return id
}

// RegisterConnectorCleanup registers a LIFO cleanup that DeleteConnectors id.
// Tests that create a connector via `h.Run("connector","create",…)` (rather
// than the raw-RPC CreateConnector helper) call this after parsing the id
// out of stdout so the ledger still owns the teardown.
func (h *H) RegisterConnectorCleanup(id int) {
	h.Register(func() { h.deleteConnector(id) })
}

// RegisterConnectorCleanupByName registers a best-effort cleanup that, at End
// time, lists connectors and deletes any whose name equals `name`. Tests that
// drive `connector create` through the CLI register this BEFORE invoking the
// command so an orphan can't escape if id-extraction from stdout fails after
// the server has already created the row. The id-based cleanup registered
// after a successful parse runs first (LIFO); this helper then no-ops on the
// follow-up list because the row is already gone.
func (h *H) RegisterConnectorCleanupByName(name string) {
	h.Register(func() { h.deleteConnectorByName(name) })
}

func (h *H) deleteConnectorByName(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		Connectors []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"connectors"`
	}
	const listPath = "/rpc/public/textql.rpc.public.connector.ConnectorService/GetConnectors"
	if err := h.client.Unary(ctx, listPath, struct{}{}, &resp); err != nil {
		// Non-fatal: the by-name cleanup is a safety net. A list failure here
		// most likely means the test was already in trouble; surface but don't
		// fail cleanup itself.
		h.t.Logf("cleanup-by-name list connectors (%s): %v", name, err)
		return
	}
	for _, c := range resp.Connectors {
		if c.Name != name {
			continue
		}
		const delPath = "/rpc/public/textql.rpc.public.connector.ConnectorService/DeleteConnector"
		if err := h.client.Unary(ctx, delPath, map[string]any{"connectorId": c.ID}, nil); err != nil {
			h.t.Errorf("cleanup-by-name DeleteConnector(%s id=%d): %v", name, c.ID, err)
			h.RecordManualRevert(
				fmt.Sprintf("connector id=%d name=%s", c.ID, name),
				fmt.Sprintf("by-name auto-delete failed: %v", err),
			)
		}
	}
}

func (h *H) deleteConnector(id int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.connector.ConnectorService/DeleteConnector"
	if err := h.client.Unary(ctx, path, map[string]any{"connectorId": id}, nil); err != nil {
		// Cleanup errors are non-fatal for the test result (the test body may
		// already have passed), but they must be visible so a leftover gets
		// tracked down rather than silently accumulating.
		h.t.Errorf("cleanup DeleteConnector(%d): %v", id, err)
		h.RecordManualRevert(
			fmt.Sprintf("connector id=%d name=%s", id, h.Prefix),
			fmt.Sprintf("auto-delete failed: %v", err),
		)
	}
}

// RegisterAPIKeyCleanupByName registers a best-effort cleanup that, at End
// time, lists api keys and revokes any whose name equals `name`. Tests that
// drive `auth keys create` through the CLI call this BEFORE the create so an
// orphan key can't survive if the post-create lookup or assertion errors out.
func (h *H) RegisterAPIKeyCleanupByName(name string) {
	h.Register(func() { h.revokeAPIKeyByName(name) })
}

func (h *H) revokeAPIKeyByName(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		APIKeys []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apiKeys"`
	}
	const listPath = "/rpc/public/textql.rpc.public.rbac.RBACService/ListApiKeys"
	if err := h.client.Unary(ctx, listPath, struct{}{}, &resp); err != nil {
		h.t.Logf("cleanup-by-name list api keys (%s): %v", name, err)
		return
	}
	for _, k := range resp.APIKeys {
		if k.Name != name {
			continue
		}
		const revPath = "/rpc/public/textql.rpc.public.rbac.RBACService/RevokeApiKey"
		if err := h.client.Unary(ctx, revPath, map[string]any{"apiKeyId": k.ID}, nil); err != nil && !isNotFound(err) {
			h.t.Errorf("cleanup-by-name RevokeApiKey(%s id=%s): %v", name, k.ID, err)
			h.RecordManualRevert(
				fmt.Sprintf("api key id=%s name=%s", k.ID, name),
				fmt.Sprintf("by-name auto-revoke failed: %v", err),
			)
		}
	}
}

// RegisterServiceAccountCleanupByName registers a best-effort cleanup that,
// at End time, lists service accounts and deletes any whose displayName
// equals `name`. CLI-driven create tests call this before the create so a
// successful create followed by a failed post-create lookup cannot orphan
// the SA.
func (h *H) RegisterServiceAccountCleanupByName(name string) {
	h.Register(func() { h.deleteServiceAccountByName(name) })
}

func (h *H) deleteServiceAccountByName(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		ServiceAccounts []struct {
			MemberID    string `json:"memberId"`
			DisplayName string `json:"displayName"`
		} `json:"serviceAccounts"`
	}
	const listPath = "/rpc/public/textql.rpc.public.rbac.RBACService/ListServiceAccounts"
	if err := h.client.Unary(ctx, listPath, struct{}{}, &resp); err != nil {
		h.t.Logf("cleanup-by-name list service accounts (%s): %v", name, err)
		return
	}
	for _, sa := range resp.ServiceAccounts {
		if sa.DisplayName != name {
			continue
		}
		const delPath = "/rpc/public/textql.rpc.public.rbac.RBACService/DeleteServiceAccount"
		if err := h.client.Unary(ctx, delPath, map[string]any{"memberId": sa.MemberID}, nil); err != nil && !isNotFound(err) {
			h.t.Errorf("cleanup-by-name DeleteServiceAccount(%s memberId=%s): %v", name, sa.MemberID, err)
			h.RecordManualRevert(
				fmt.Sprintf("service account memberId=%s name=%s", sa.MemberID, name),
				fmt.Sprintf("by-name auto-delete failed: %v", err),
			)
		}
	}
}

// CreateChat posts CreateChat bound to the given connector ids, returning
// the chat id. Cleanup cascades to any child messages, shares, etc.
func (h *H) CreateChat(suffix string, connectorIDs []int) string {
	h.t.Helper()
	if h.env.dryRun {
		h.t.Logf("dryrun: create chat with connectors %v", connectorIDs)
		return ""
	}
	req := map[string]any{
		"paradigm": map[string]any{
			"type":    "TYPE_UNIVERSAL",
			"version": 1,
			"options": map[string]any{
				"universal": map[string]any{
					"connectorIds":     connectorIDs,
					"webSearchEnabled": true,
					"sqlEnabled":       true,
					"pythonEnabled":    true,
				},
			},
		},
		"model":       "MODEL_DEFAULT",
		"methodology": "METHODOLOGY_ADAPTIVE",
		"summary":     h.ResourceName(suffix),
	}
	var resp struct {
		Chat struct {
			ID string `json:"id"`
		} `json:"chat"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.chat.ChatService/CreateChat"
	if err := h.client.Unary(ctx, path, req, &resp); err != nil {
		h.t.Fatalf("CreateChat: %v", err)
	}
	id := resp.Chat.ID
	h.Register(func() { h.deleteChat(id) })
	return id
}

func (h *H) deleteChat(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.chat.ChatService/DeleteChat"
	if err := h.client.Unary(ctx, path, map[string]any{"chatId": id}, nil); err != nil {
		h.t.Errorf("cleanup DeleteChat(%s): %v", id, err)
		h.RecordManualRevert(
			fmt.Sprintf("chat id=%s", id),
			fmt.Sprintf("auto-delete failed: %v", err),
		)
	}
}

// APIKeyHandle is returned from CreateAPIKey: the key id plus the one-shot
// plaintext token the server emits. The token is not persisted by the
// harness — callers that want to use it must capture it before End fires.
type APIKeyHandle struct {
	ID    string
	Token string
}

// CreateAPIKey posts CreateApiKey and defers RevokeApiKey on the latest id
// (the id may be rotated mid-test; Rotate swaps the deferred id too).
func (h *H) CreateAPIKey(suffix string) APIKeyHandle {
	h.t.Helper()
	name := h.ResourceName(suffix)
	if h.env.dryRun {
		h.t.Logf("dryrun: create api key %q", name)
		return APIKeyHandle{}
	}
	// Server rejects CreateApiKey without either assumedRoles or
	// inheritAllRoles=true (Error 207). The e2e suite takes the simpler
	// "inherit all member roles" path — matches the webapp's default.
	req := map[string]any{"name": name, "inheritAllRoles": true}
	var resp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
		APIKeyHash string `json:"apiKeyHash"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.rbac.RBACService/CreateApiKey"
	if err := h.client.Unary(ctx, path, req, &resp); err != nil {
		h.t.Fatalf("CreateApiKey: %v", err)
	}
	// Box the id in a pointer so Rotate can update the deferred target
	// without registering a second cleanup.
	id := resp.APIKey.ID
	current := &id
	h.Register(func() { h.revokeAPIKey(*current) })
	h.latestKey = current
	return APIKeyHandle{ID: id, Token: resp.APIKeyHash}
}

// RotateAPIKey rotates the key most recently created by CreateAPIKey and
// swaps the deferred revoke to point at the successor.
func (h *H) RotateAPIKey(id string) APIKeyHandle {
	h.t.Helper()
	if h.env.dryRun {
		h.t.Logf("dryrun: rotate api key %s", id)
		return APIKeyHandle{}
	}
	req := map[string]any{"apiKeyId": id}
	var resp struct {
		APIKey struct {
			ID string `json:"id"`
		} `json:"apiKey"`
		APIKeyHash string `json:"apiKeyHash"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.rbac.RBACService/RotateApiKey"
	if err := h.client.Unary(ctx, path, req, &resp); err != nil {
		h.t.Fatalf("RotateApiKey: %v", err)
	}
	if h.latestKey != nil {
		// Redirect the existing deferred cleanup to the rotated id. The old
		// id is invalidated server-side by the rotate itself.
		*h.latestKey = resp.APIKey.ID
	}
	return APIKeyHandle{ID: resp.APIKey.ID, Token: resp.APIKeyHash}
}

func (h *H) revokeAPIKey(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.rbac.RBACService/RevokeApiKey"
	if err := h.client.Unary(ctx, path, map[string]any{"apiKeyId": id}, nil); err != nil {
		// A 404/not-found on the old id after a rotate is expected and not an
		// error we want to ledger — skip those; everything else surfaces.
		if isNotFound(err) {
			return
		}
		h.t.Errorf("cleanup RevokeApiKey(%s): %v", id, err)
		h.RecordManualRevert(
			fmt.Sprintf("api key id=%s", id),
			fmt.Sprintf("auto-revoke failed: %v", err),
		)
	}
}

// CreateServiceAccount posts CreateServiceAccount and defers DeleteServiceAccount.
// Server mandates either assumedRoles or inheritAllRoles=true AND a real
// ownerMemberId (Error 207), so we pre-fetch the caller's memberId via
// GetMember and inherit the caller's roles rather than plumbing role UUIDs
// through the suite.
func (h *H) CreateServiceAccount(suffix string) string {
	h.t.Helper()
	name := h.ResourceName(suffix)
	if h.env.dryRun {
		h.t.Logf("dryrun: create service account %q", name)
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var member struct {
		Member struct {
			MemberID string `json:"memberId"`
		} `json:"member"`
	}
	const memberPath = "/rpc/public/textql.rpc.public.auth.PublicAuthService/GetMember"
	if err := h.client.Unary(ctx, memberPath, struct{}{}, &member); err != nil {
		h.t.Fatalf("CreateServiceAccount: resolve caller memberId: %v", err)
	}
	// The error message mentions `assumed_roles` / `inherit_all_roles`, but
	// the camelCase field this proto actually accepts is `roleIds` — the
	// snake-case form and `assumedRoles` both fall through to the same
	// validation error. Inherit the caller's roles so the SA can't outrank
	// the creator.
	var perms struct {
		Roles []struct {
			ID string `json:"id"`
		} `json:"roles"`
	}
	const permsPath = "/rpc/public/textql.rpc.public.rbac.RBACService/GetCurrentMemberRolesAndPermissions"
	if err := h.client.Unary(ctx, permsPath, struct{}{}, &perms); err != nil {
		h.t.Fatalf("CreateServiceAccount: resolve caller roles: %v", err)
	}
	roleIDs := make([]string, 0, len(perms.Roles))
	for _, r := range perms.Roles {
		roleIDs = append(roleIDs, r.ID)
	}
	if len(roleIDs) == 0 {
		h.t.Fatalf("CreateServiceAccount: caller has zero roles — cannot inherit")
	}
	req := map[string]any{
		"name":          name,
		"ownerMemberId": member.Member.MemberID,
		"roleIds":       roleIDs,
	}
	var resp struct {
		MemberID string `json:"memberId"`
	}
	const path = "/rpc/public/textql.rpc.public.rbac.RBACService/CreateServiceAccount"
	if err := h.client.Unary(ctx, path, req, &resp); err != nil {
		h.t.Fatalf("CreateServiceAccount: %v", err)
	}
	id := resp.MemberID
	h.Register(func() { h.deleteServiceAccount(id) })
	return id
}

func (h *H) deleteServiceAccount(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const path = "/rpc/public/textql.rpc.public.rbac.RBACService/DeleteServiceAccount"
	if err := h.client.Unary(ctx, path, map[string]any{"memberId": id}, nil); err != nil {
		h.t.Errorf("cleanup DeleteServiceAccount(%s): %v", id, err)
		h.RecordManualRevert(
			fmt.Sprintf("service account memberId=%s", id),
			fmt.Sprintf("auto-delete failed: %v", err),
		)
	}
}

// isNotFound reports whether err looks like a 404/NotFound response. Used by
// cleanups that tolerate "already gone" (e.g. post-rotate revoke).
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "notfound")
}
