# TextQL features & domain entities

Human-readable inventory, maintained by the `textql-webapp-probe` skill. Last updated: 2026-04-17.

Per-endpoint request/response schemas live in `api-catalog/` (~85 endpoints as of 2026-04-17).

## API shape (global)

- **Style:** Connect-RPC (buf-connect). All calls are `POST https://app.textql.com/rpc/public/<fully.qualified.Service>/<Method>`.
- **Content-Type:** `application/json` request + response.
- **Field casing:** protobuf JSON — **camelCase only**. Sending both `chatId` and `chat_id` → 400 `"duplicate field"`. CLI must emit camelCase.
- **Error shape:** `{"code": "<lowercase_code>", "message": "<text>"}` (e.g. `invalid_argument`, `not_found`, `internal`).
- **IDs:** mixed. Members/orgs/chats/dashboards use UUIDs; ontologies/connectors use integers.
- **Service namespace:** `textql.rpc.public.<area>.<Service>` (e.g. `textql.rpc.public.chat.ChatService`).
- **Frontend:** SvelteKit. Page data loaders hit `/<route>/__data.json`. Version poll at `/_notApp/version.json` every ~10s.
- **Streaming RPCs:** some methods are long-lived POSTs (server-streaming). Observed: `chat.ChatService/StreamChat`, `notifications.NotificationService/StreamNotifications`, `feed.FeedService/StreamFeed`.
  - Wire format (verified on StreamChat): Connect-RPC envelope framing — `[flags:1B][length:4B big-endian][JSON payload]`, repeated. Trailer frame carries `flags=2`. Client sends a single framed request (chatId, latestCompleteCellId, research, model). Server emits one frame per cell update with `lifecycle` transitioning `LIFECYCLE_CREATING` → `LIFECYCLE_CREATED` → `LIFECYCLE_EXECUTED`.
- **Analytics (ignore for CLI):** `hermes.textql.com` — self-hosted PostHog (capture + feature flags). Never call from CLI.
- **Auth bootstrap (browser OAuth, not for CLI):** `PublicAuthService.GetGoogleOAuthUrl` → `HandleGoogleOAuthCallback` → `ValidateIntermediaryToken` → `ExchangeIntermediaryToken` → `GetOrganization` / `GetMember`. Session cookie-based; the CLI should not drive this path.
- **CLI auth (verified 2026-04-17):**
  - **Path:** `rbac.RBACService/CreateApiKey` returns `{apiKey:{...}, apiKeyHash}`. The `apiKeyHash` field is misleadingly named — it is NOT a hash, it is the one-time plaintext token the CLI must save. Format: `base64(<memberId>:<hex-secret>)`. If you lose it, rotate (invalidates old, issues new plaintext) or create a fresh key.
  - **Header:** `Authorization: Bearer <apiKeyHash>`. Verified against `/GetMember` from curl with no browser cookies → 200.
  - **Failure modes:** no header / missing key → 401 `{"code":"unauthenticated","message":"authentication required"}`. Revoked or expired key → 401 `{"error":{"code":401,"status":"Unauthorized","message":"The request could not be authorized"}}` (note different error envelope, suggesting the reverse-proxy layer rather than the RPC layer is the one rejecting).
  - **Scope of inherited perms:** `inheritAllRoles:true` on CreateApiKey grants the caller's assumedRoles to the key. For service-account keys, the roles come from the SA's immutable roleIds.
  - **Do NOT use:** `secret.SecretService/ListApiAccessKeys` — that's connector-scoped OAuth/API credentials, not platform auth.
- **Last verified:** 2026-04-17

## Services observed (19)

`audit_log.AuditLogService`, `auth.PublicAuthService`, `chat.ChatService`, `connector.ConnectorService`, `context_prompts.ContextPromptsService`, `dashboard.DashboardService`, `dataset.DatasetService`, `engagement.EngagementService`, `feed.FeedService`, `notifications.NotificationService`, `ontology.OntologyService`, `packages.OrgPackageService`, `playbook.PlaybookService`, `rbac.RBACService`, `scim.ScimService`, `secret.SecretService`, `settings.SettingsService`, `sharing.SharingService`, `slack.SlackService`.

## Auth & organization

- **Service:** `auth.PublicAuthService`
- **Methods:** `GetGoogleOAuthUrl`, `HandleGoogleOAuthCallback`, `ValidateIntermediaryToken`, `ExchangeIntermediaryToken`, `GetOrganization`, `GetMember`, `GetMemberInOrgById`, `GetOrgOIDCProviders`, `ListOrganizations`.
- **Notes:** Users belong to 1+ organizations (current: "High Performance Technologies"). OAuth via Google with intermediary-token handoff. OIDC providers configurable per org. `GetMember` returns self; `GetMemberInOrgById {memberId}` resolves any member by UUID and returns same shape.
- **Last verified:** 2026-04-17

## Chats / Threads

- **Entity:** `Chat` (UI: "Thread"). UUID-keyed. Routes `/chat/new`, `/chat/<uuid>`, list `/chats`.
- **Service:** `chat.ChatService`
- **Methods:** `GetChats`, `GetChat`, `GetChatHistory`, `GetChatArtifactsSummary`, `GetMembersWithChats`, `CheckChatPermissions`, `MarkChatRead`, `CreateChat`, `SendMessage`, `StreamChat` (server-stream), `GetLlmUsage`, `GetThreadWarnings`, `GetBillingStats`, `GetObservabilityStats`, `GetBackfillStatus`, `GetBackfillPreview`, `BookmarkChat`, `UnbookmarkChat`, `UpdateChat`, `DuplicateChat`, `DeleteChat`.
- **Mutations (verified 2026-04-17):**
  - `BookmarkChat {chatId}` / `UnbookmarkChat {chatId}` → `{}`.
  - `UpdateChat` — used for rename; req `{chatId, summary: "<new name>"}`. The display name is stored in the `summary` field (not `name`).
  - `DuplicateChat {chatId}` → `{chat: {id: <new uuid>, ...}}`.
  - `DeleteChat {chatId}` → `{}`.
- **Notes:** A chat scopes to a connector and/or dataset. Artifacts (dashboards, reports, snippets) are emitted into the thread.
- **Send-flow (verified 2026-04-17):**
  1. `CreateChat` — req `{paradigm:{type, version, options:{universal:{connectorIds:[int], webSearchEnabled, sqlEnabled, pythonEnabled, playbookToolsEnabled}}}, model, research, dashboardMode, methodology}`. Resp `{chat:{id (UUID), paradigm, ...}}`. Creates empty thread.
     - Observed enums: `paradigm.type = "TYPE_UNIVERSAL"`, `model = "MODEL_DEFAULT"`, `methodology = "METHODOLOGY_ADAPTIVE"`.
     - `paradigm.version` is an integer (observed `1`) — schema version for `paradigm.options`.
  2. `SendMessage` — req `{chatId, message, messageId}` (messageId is client-generated UUID = user-cell id). Resp `{cellId}` echoes that id.
  3. `StreamChat` — req `{chatId, latestCompleteCellId, research, model}` (framed). Server streams cell updates until assistant cell reaches `LIFECYCLE_CREATED`. Each frame is a full cell snapshot; correlate by `id`.
  4. `MarkChatRead {chatId}` — fired on view. Resp `{}`.
  5. `engagement.EngagementService/RecordEngagement` — `{eventType:"ENGAGEMENT_TYPE_VIEW", primitiveId:<chatId>, primitiveType:"PRIMITIVE_TYPE_CHAT"}`. Resp `{}`.
  6. `GetChatHistory {chatId}` — canonical replay; returns `{cells:[{id, timestamp, complete, generated?, lifecycle, senderMemberId?, mdCell:{content, renderedHtml}, summaryCell?, ...}]}`.
- **Cell shape:** one `id` per cell. Variant payload fields observed: `mdCell{content, renderedHtml}` (markdown), `summaryCell{summary}` (agent chain-of-thought / tool summary), `statusCell` (transient working status), `pyCell{code, executionTimeMs (string)}` (agent-authored Python executed in a sandbox; when connector-scoped the code has pre-bound variables `_tql_<slug>` e.g. `_tql_connectors`, `_tql_slack_channels`), `playbookEditorCell{action, errorMessage}` (playbook-editor tool result — enum `action` observed `PLAYBOOK_ACTION_LIST`), and `sqlCell` (captured in prior runs).
- **Tool-selection in chat create:** tool gating lives inside `paradigm.options.universal` (NOT at the top level). Keys: `connectorIds:[<int>]` scopes the chat to one or more connectors; `sqlEnabled`, `pythonEnabled`, `webSearchEnabled`, `playbookToolsEnabled` gate which tool-cells the agent can emit. A chat created without `connectorIds` can still run (web-search / reasoning) but sending a message to it triggers a client-side "No Connector Selected" dialog.
- **Model enums:** request uses `MODEL_DEFAULT`; LLM usage response names specific models (`MODEL_HAIKU_4_5`, `MODEL_SONNET_4_6`) — agent tiers the call internally.
- **Last verified:** 2026-04-17

## Dashboards

- **Entity:** `Dashboard` — persisted Streamlit-backed visualization; foldered. Each dashboard has Python source (`code` field) that runs in a per-dashboard Streamlit pod reachable via `streamlitUrl` / `embedUrl`.
- **Service:** `dashboard.DashboardService`
- **Methods:** `ListDashboards`, `ListDashboardFolders`, `GetMembersWithDashboards`, `GetDashboard`, `SpawnDashboard`, `CheckDashboardHealth`.
- **Detail flow on `/dashboard/<uuid>`:**
  1. `GetDashboard {dashboardId}` — returns `{dashboard: {id, orgId, creatorId, name, code (streamlit Python source), ...}}`.
  2. `SpawnDashboard {dashboardId}` — starts/warms the Streamlit pod, returns `{refreshedAt}`.
  3. `CheckDashboardHealth {dashboardIds:[...]}` — polls status: `{dashboards:[{dashboardId, status:"HEALTH_STATUS_HEALTHY", streamlitUrl, embedUrl:"/sandbox/proxy/<org>-dashboard-<id>/8501/", refreshedAt, code}]}`.
- **Routes:** `/dashboards` (list), `/dashboard/<uuid>` (detail).
- **Last verified:** 2026-04-17

## Playbooks

- **Entity:** `Playbook` — scheduled / recurring agent workflows. Bound to a connector (`connectorId`). Stores a prompt, schedule (cron), and activation state. Also emits "chat reports" from each run.
- **Service:** `playbook.PlaybookService`
- **Methods:** `GetPlaybooks`, `GetChatReportsSummary`, `GetPlaybook`, `GetPlaybookLineage`, `GetPlaybookReports`.
- **Detail flow on `/playbooks/<uuid>`:**
  1. `GetPlaybook {playbookId}` — returns `{playbook: {id, orgId, memberId, connectorId (int), name, prompt, schedule, active, createdAt, updatedAt, ...}}`.
  2. `GetPlaybookLineage {playbookId}` — lineage graph (empty in sample).
  3. `GetPlaybookReports {playbookId}` — report history.
- **Routes:** `/playbooks` (list), `/playbooks/<uuid>` (detail).
- **Last verified:** 2026-04-17

## Feed

- **Entity:** shared activity feed — mentions, leaderboard, agent activity.
- **Service:** `feed.FeedService`
- **Methods:** `GetFeed`, `StreamFeed` (server-stream), `GetFeedStats`, `GetLeaderboard`, `ListMentionableUsers`, `ListUserAgents`.
- **Route:** `/feed`.
- **Last verified:** 2026-04-17

## Notifications

- **Service:** `notifications.NotificationService`
- **Methods:** `GetNotifications`, `StreamNotifications` (server-stream, opened on every page).
- **Last verified:** 2026-04-17

## Connectors

- **Entity:** `Connector` — data-source binding (Tableau, Databricks, Zoom, Zoho, Neo4j, AWS, HPT, example orgs, etc.). Integer `connectorId`.
- **Service:** `connector.ConnectorService`
- **Methods:** `GetConnectors`, `GetExampleQueries`, `ListConnectorTables`, `TestConnector`, `CreateConnector`, `GetConnector`, `UpdateConnector`, `DeleteConnector`.
- **CRUD (verified 2026-04-17, Postgres dialect):**
  - Request shape uses a protobuf oneof nested under `config`: `{config: {connectorType: "POSTGRES", name, postgres: {host, port, user, password, database, dialect, sslMode}}}`. Other dialect variants (bigquery/snowflake/redshift/mysql/sqlserver/databricks/supabase/motherduck) almost certainly follow the same nesting — unverified.
  - `TestConnector` is non-blocking: 200 with `{error: "<driver error>"}` on failure; server does NOT require a passing test before Create. UI gates Create behind Test but server will happily create an unreachable connector.
  - `CreateConnector` returns `{connectorId: <int>, name, connectorType}`.
  - `UpdateConnector` requires top-level `connectorId` alongside `config` (putting id inside config → 500). Returns `{connector: {id, name, connectorType, memberId, createdAt, <dialect>Metadata: {...}, authStrategy}}` — password omitted.
  - `GetConnector {connectorId}` returns same connector shape; `{id}` returns 404.
  - `DeleteConnector {connectorId}` → `{success: true}`.
- **Route:** `/connectors`.
- **Last verified:** 2026-04-17

## Datasets

- **Entity:** `Dataset` — queryable collection scoped to a connector (e.g. "Superstore Dataset" under `tableau.hpt.tools`).
- **Service:** `dataset.DatasetService`
- **Methods:** `GetDatasets` (called with different scopes).
- **Last verified:** 2026-04-17

## Ontology

- **Entity:** `Ontology` — semantic layer over connectors/datasets.
- **Service:** `ontology.OntologyService`
- **Methods:** `GetOntologies`, `GetOntologiesSummary`, `GetOntologyById`.
- **Route:** `/ontology`.
- **Last verified:** 2026-04-17

## Context Library

- **Entity:** `ContextPrompt` — org-wide prompt snippets; "ledger" tracks proposed changes.
- **Service:** `context_prompts.ContextPromptsService`
- **Methods:** `ListAllOrgContextPrompts`, `ListLedgerChangeProposals`.
- **Route:** `/context` (opens with `?context_id=<uuid>` selected).
- **Last verified:** 2026-04-17

## Observability

- **Covered by chat service.** Route `/observability` aggregates `chat.ChatService/GetObservabilityStats`, `GetBillingStats`, `GetThreadWarnings`, `GetBackfillStatus`, `GetBackfillPreview`, `GetLlmUsage`, plus `feed.FeedService/ListUserAgents` and `settings.SettingsService/ListOrganizationMembers`.
- **Notes:** Backfill endpoints hint at historical-data ingestion jobs visible to admins.
- **Last verified:** 2026-04-17

## Engagement

- **Service:** `engagement.EngagementService`
- **Methods:** `RecordEngagement` — req `{eventType, primitiveId, primitiveType}`, resp `{}`.
- **Enums seen:** `eventType=ENGAGEMENT_TYPE_VIEW`; `primitiveType=PRIMITIVE_TYPE_CHAT`. Other primitives (dashboard, playbook, report) likely follow the same shape — confirm by opening those surfaces.
- **Notes:** First-party engagement tracking (separate from Hermes/PostHog). Always accompanies a chat view together with `MarkChatRead`.
- **Last verified:** 2026-04-17

## Settings

- **Route:** `/settings` with hash-tabs: `#personal`, `#members`, `#roles`, `#appearance`, `#feat`, `#security`, `#packages`, `#dev`, `#audit`.
- **Service:** `settings.SettingsService`
- **Methods:** `CheckMemberStatus`, `GetModelDeprecations`, `ListOrganizationMembers`.
- **Last verified:** 2026-04-17

## RBAC (roles, permissions, API keys, service accounts)

- **Service:** `rbac.RBACService`
- **Methods:** `GetCurrentMemberRolesAndPermissions`, `GetMemberRoles`, `ListRoles`, `ListPermissions`, `ListApiKeys`, `ListServiceAccounts`, `CreateApiKey`, `RotateApiKey`, `RevokeApiKey`, `CreateServiceAccount`, `DeleteServiceAccount`.
- **API key CRUD (verified 2026-04-17):**
  - `CreateApiKey` req `{name, expirySeconds (int), inheritAllRoles: bool}` → `{apiKey: {id, memberId, name, createdAt, expiresAt, apiKeyShort, assumedRoles:[...]}}`. Plaintext key is one-time — shown once, captured via the creation response and never retrievable again (list returns only `apiKeyShort`).
  - `RotateApiKey {apiKeyId}` → `{apiKey: {...}}` with a new `id`; old key revoked immediately.
  - `RevokeApiKey {apiKeyId}` → `{success: true}`. Idempotent.
  - `ListApiKeys` req `{scope: "API_KEY_SCOPE_ALL", includeRevoked: bool, sortBy: "API_KEY_SORT_FIELD_CREATED_AT", sortDirection: "SORT_DIRECTION_DESC", pageSize: int}`.
- **Service account CRUD (verified 2026-04-17):**
  - `CreateServiceAccount` req `{name, ownerMemberId, roleIds:[<uuid>]}` → `{memberId, email}`. Email auto-generated as `<slug>@embed.textql`. Roles are immutable on the account — "create a new one to change roles".
  - `DeleteServiceAccount {memberId}` — cascades and revokes every API key associated with the account.
  - `ListServiceAccounts`, `GetMemberRoles {memberIds:[<uuid>]}`.
- **Notes:** Full RBAC system. API keys and service accounts live here — almost certainly the CLI-auth path.
- **Last verified:** 2026-04-17

## Secrets

- **Service:** `secret.SecretService`
- **Methods:** `ListApiAccessKeys`, `ListApiProviders`.
- **Notes:** Distinct from `rbac.ListApiKeys`. `ListApiProviders` returns the catalog of API connector providers (e.g. `notion`) with `{id, name, iconUrl, authType, description, docsUrl}` — fires on opening the New Connector dialog. `ListApiAccessKeys` is likely per-provider connector credentials rather than platform auth. Confirm before using.
- **Last verified:** 2026-04-17

## Audit log

- **Service:** `audit_log.AuditLogService`
- **Methods:** `ListAuditLogs`.
- **Route:** `/settings#audit`.
- **Last verified:** 2026-04-17

## Packages

- **Service:** `packages.OrgPackageService`
- **Methods:** `ListOrgPackages`.
- **Notes:** "Packages" tab in settings; likely bundled connector/ontology/playbook assets distributed to orgs.
- **Last verified:** 2026-04-17

## SCIM (provisioning)

- **Service:** `scim.ScimService`
- **Methods:** `ListScimOAuthClients`, `ListScimTokens`.
- **Notes:** Enterprise SSO/SCIM provisioning. Out of scope for CLI unless admin tooling.
- **Last verified:** 2026-04-17

## Sharing

- **Service:** `sharing.SharingService`
- **Methods:** `CreateShare`.
- **Notes:** `CreateShare` req `{primitiveId, primitiveType, channel}`. Enums observed: `primitiveType="PRIMITIVE_TYPE_CHAT"` (others likely mirror engagement primitives), `channel="SHARE_CHANNEL_LINK_COPY"`. Resp `{share: {id, shareToken, ...}, url}` — the `url` is a ready-to-share token link.
- **Last verified:** 2026-04-17

## Slack integration

- **Service:** `slack.SlackService`
- **Methods:** `ListInstallations`, `GetCurrentUser`, `ListUsers`, `ListChannels`.
- **Notes:** Per-org Slack workspace binding (used to @-mention Slack users and pipe playbook reports to channels). `ListInstallations` returns `{installations:[{teamId, createdAt, name}]}`; `GetCurrentUser` resolves the browser-session user to a Slack user; `ListUsers`/`ListChannels` expose workspace roster + channel list for mention/destination pickers. Fires on the playbook detail page and mention pickers.
- **Last verified:** 2026-04-17
