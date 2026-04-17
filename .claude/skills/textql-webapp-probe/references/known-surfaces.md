# Known surfaces

Pages and flows that have already been probed. Append a one-line entry after each successful probe so future runs can tell what's covered.

Format: `- YYYY-MM-DD — <page or flow> — <catalog entries touched>`

<!-- entries below -->
- 2026-04-17 — `/chat/new` root/sidebar baseline — services: auth, chat, dashboard, playbook, feed, notifications, connector, dataset, ontology, settings, rbac, secret
- 2026-04-17 — `/connectors` — no new methods beyond baseline
- 2026-04-17 — `/ontology` — +connector.ListConnectorTables, +ontology.GetOntologies, +ontology.GetOntologyById
- 2026-04-17 — `/dashboards` — +dashboard.ListDashboardFolders, +dashboard.GetMembersWithDashboards
- 2026-04-17 — `/playbooks` — no new methods
- 2026-04-17 — `/feed` — +feed.StreamFeed, +feed.GetFeedStats, +feed.GetLeaderboard, +feed.ListMentionableUsers
- 2026-04-17 — `/context` — +context_prompts.* (new service)
- 2026-04-17 — `/observability` — +chat.GetObservabilityStats/GetBillingStats/GetBackfillStatus/GetBackfillPreview/GetThreadWarnings, +feed.ListUserAgents, +settings.ListOrganizationMembers
- 2026-04-17 — `/settings#dev` — +audit_log.*, +packages.*, +rbac.ListApiKeys/ListServiceAccounts/GetMemberRoles, +scim.* (new services)
- 2026-04-17 — `/chat/<uuid>` — +chat.GetChat/GetChatHistory/GetChatArtifactsSummary/StreamChat/CheckChatPermissions/MarkChatRead/GetLlmUsage, +engagement.RecordEngagement, +playbook.GetChatReportsSummary
- 2026-04-17 — full catalog pass via in-page `fetch` (patched) — 50 endpoints captured with req+resp bodies, written to `api-catalog/`. Empty-body probe hit every list/summary endpoint; param probe resolved 10 parameterized endpoints (chat-scoped, org-scoped, connector/ontology by id). Field casing is strict camelCase. Streaming RPCs (`StreamChat`, `StreamFeed`, `StreamNotifications`) intentionally not captured.
- Not yet catalogued: write endpoints (any mutation), `MarkChatRead`, `RecordEngagement`, chat send / tool-call flow, streaming bodies, any admin mutations (create/delete api key, connector CRUD). These require triggering UI actions, not fetch replay.
- 2026-04-17 — UI-driven chat-send flow on `/chat/new` ("Chat Without Connector"): captured `chat.CreateChat` (req+resp), `chat.SendMessage` (req+resp, messageId is client-side UUID), `chat.MarkChatRead`, `engagement.RecordEngagement`, real-param `chat.GetChats` request body, `chat.StreamChat` (5 framed cell updates via Connect-RPC envelope framing `[flags:1B][len:4B BE][json]`), `chat.GetChatHistory` and `chat.GetLlmUsage` with real chatId. Catalog entries overwritten. `secret.ListApiAccessKeys` on this org returns `{}` — no API access keys configured.
- Still not catalogued: tool-invoking agent flows (artifact cells, SQL cells, dashboards emitted into a thread), bookmark/delete/rename chat mutations, dashboard detail page RPCs, playbook detail/run RPCs, API-key create/rotate (`rbac.*`), connector CRUD.
- 2026-04-17 — `/dashboard/<uuid>` — +dashboard.GetDashboard/SpawnDashboard/CheckDashboardHealth. Streamlit-backed: each dashboard has Python `code` + per-pod `streamlitUrl`/`embedUrl` at `/sandbox/proxy/<org>-dashboard-<id>/8501/`.
- 2026-04-17 — `/playbooks/<uuid>` — +playbook.GetPlaybook/GetPlaybookLineage/GetPlaybookReports. Discovered new service `slack.SlackService` (4 methods: ListInstallations, GetCurrentUser, ListUsers, ListChannels) — fires on playbook detail for mention/destination pickers.
- 2026-04-17 — direct-fetch probes (on dashboard page) filled in chat detail endpoints: `chat.GetChat`, `chat.GetChatArtifactsSummary`, `chat.CheckChatPermissions`, `chat.GetThreadWarnings` with real chatId. All require camelCase IDs: `chatId`/`dashboardId`/`playbookId` (server error messages use snake_case hints but request must be camelCase).
- Service count 17 → 18 (added `slack.SlackService`). Total catalog entries ~65.
- 2026-04-17 — `/chats` right-click menu mutations — +chat.BookmarkChat, +chat.UnbookmarkChat, +chat.UpdateChat (rename via `summary` field), +chat.DuplicateChat, +chat.DeleteChat, +sharing.SharingService/CreateShare (new service, 19th). Also captured rbac.{ListRoles, ListPermissions, GetObjectAccess, ListAccessRequests, GetCurrentMemberRolesAndPermissions} via the Share dialog.
- 2026-04-17 — tool-invoking chat on `/chat/new` (connector 608=HPT) — captured CreateChat with `paradigm.options.universal.connectorIds:[int]` + `{sqlEnabled,pythonEnabled,webSearchEnabled}` flags; StreamChat cell variants now include `statusCell`, `pyCell{code, executionTimeMs:string}` with `_tql_<slug>` pre-bound variables, `playbookEditorCell{action:"PLAYBOOK_ACTION_LIST", errorMessage}`, `mdCell`, `summaryCell`. `connector.GetExampleQueries` captured.
- 2026-04-17 — `/settings#dev` CRUD — +rbac.{CreateApiKey, RotateApiKey, RevokeApiKey, CreateServiceAccount, DeleteServiceAccount}, +audit_log.ListAuditLogs with real action strings (`api_key.{created,rotated,revoked}`, `service_account.{created,deleted}`, authMethod="oathkeeper"). ApiKey plaintext is one-time in the Create response; list returns only `apiKeyShort`. Service account roles are immutable post-create. DeleteServiceAccount cascades to revoke all associated API keys.
- 2026-04-17 — `/connectors` CRUD (Postgres dialect) — +connector.{TestConnector, CreateConnector, GetConnector, UpdateConnector, DeleteConnector}, +secret.ListApiProviders. Request shape uses a protobuf oneof nested under `config`: `{config:{connectorType:"POSTGRES", name, postgres:{host,port,user,password,database,dialect,sslMode}}}`. UpdateConnector requires top-level `connectorId` alongside `config` (putting id inside config → 500 "could not find connector"). CreateConnector does NOT require a passing TestConnector — server creates an unreachable connector with 200. connectorId is an integer. Test connector 6445 was created and deleted during probe.
- Service count 18 → 19 (added `sharing.SharingService`). Total catalog entries ~80.
