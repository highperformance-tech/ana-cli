# api-catalog

One JSON entry per captured Connect-RPC endpoint. Filename format: `POST_rpc__public__textql_rpc_public_<service>_<Service>__<Method>.json`. Entry schema in `.claude/skills/textql-webapp-probe/references/catalog-schema.md` (method, pathTemplate, host, lastVerified, samples[], notes[], inferredRequestSchema, inferredResponseSchema).

**All entries are anonymized.** UUIDs are placeholders (`00000000-0000-0000-0000-<seq>`), emails are `user-<seq>@example.com`, Slack IDs are `<prefix>REDACTED<seq>`, free-text prose is `"<REDACTED>"`. Before committing a new or updated entry, run `python3 .claude/skills/textql-webapp-probe/scripts/anonymize_catalog.py <file>` — `.git/hooks/pre-commit` runs the same script in `--check` mode and blocks commits with raw PII.

~95 entries across 21 services. Group summary:

- `auth_PublicAuthService__*` — GetMember, GetMemberInOrgById, GetOrganization, GetOrgOIDCProviders, ListOrganizations, ExchangeIntermediaryToken, ExchangeSession, GetGoogleOAuthUrl, HandleGoogleOAuthCallback, ValidateIntermediaryToken.
- `chat_ChatService__*` — CRUD + streaming: CreateChat, SendMessage, StreamChat, GetChat(s), GetChatHistory, UpdateChat (rename via `summary`), Bookmark/Unbookmark, Duplicate, Delete, MarkChatRead, Get{Artifacts,Thread,Llm,Observability,Billing,Backfill}*.
- `connector_ConnectorService__*` — Test/Create/Get/Update/Delete, GetConnectors, ListConnectorTables, GetExampleQueries. Verified dialects: Postgres (password), Snowflake (password / keypair / oauth-sso / oauth-individual), Databricks (access-token / client-credentials / oauth-sso / oauth-individual). Per-dialect create captures live at `POST_connector.create.<dialect>.<auth-mode>.json`. Auth-mode discrimination varies: Snowflake uses populated-field discrimination for non-OAuth modes (both `service_role`), Databricks uses a nested `databricksAuth` one-of. OAuth modes share the `oauth_sso` / `per_member_oauth` `authStrategy` tags across dialects.
- `snowflake_oauth_SnowflakeOAuthService__*` — GetSnowflakeOAuthURL. Fires when user clicks "Authenticate with Snowflake" during OAuth connector setup; returns a redirect URL with a hardcoded `app.textql.com/auth/snowflake/callback` redirect URI (CLI cannot receive the callback).
- `databricks_oauth_DatabricksOAuthService__*` — GetDatabricksOAuthURL. Analogous to the Snowflake variant; PKCE S256, scopes `all-apis offline_access`. Not yet captured to a catalog file — shape documented inline in `POST_connector.create.databricks.oauth-sso.json` notes.
- `rbac_RBACService__*` — CreateApiKey (plaintext returned once as `apiKeyHash`) + RotateApiKey (public-namespace variants), Revoke/ListApiKeys, Create/Delete/ListServiceAccounts, ListRoles, ListPermissions, GetMemberRoles, GetObjectAccess, ListAccessRequests, GetCurrentMemberRolesAndPermissions.
- `dashboard_DashboardService__*` — ListDashboards, ListDashboardFolders, GetDashboard, SpawnDashboard, CheckDashboardHealth, GetMembersWithDashboards. No create/update/delete captured yet.
- `playbook_PlaybookService__*` — GetPlaybook(s), GetPlaybookLineage, GetPlaybookReports, GetChatReportsSummary. No create/update/run captured yet.
- `ontology_OntologyService__*` — GetOntologies, GetOntologiesSummary, GetOntologyById. Readonly.
- `feed_FeedService__*` — GetFeed, GetFeedStats, GetLeaderboard, ListMentionableUsers, ListUserAgents. Streaming variant not captured.
- `notifications_NotificationService__*` — GetNotifications. Streaming variant not captured.
- `sharing_SharingService__*` — CreateShare (LINK_COPY verified; Slack channel unverified).
- `slack_SlackService__*` — ListInstallations, GetCurrentUser, ListUsers, ListChannels (readonly lookups for mention/destination pickers).
- `audit_log_AuditLogService__*` — ListAuditLogs (snake_case action strings like `api_key.created`).
- `packages_OrgPackageService__*` — ListOrgPackages.
- `scim_ScimService__*` — ListScimOAuthClients, ListScimTokens. Admin-only.
- `secret_SecretService__*` — ListApiAccessKeys, ListApiProviders.
- `settings_SettingsService__*` — CheckMemberStatus, GetModelDeprecations, ListOrganizationMembers.
- `context_prompts_ContextPromptsService__*` — ListAllOrgContextPrompts, ListLedgerChangeProposals. Readonly.
- `dataset_DatasetService__*` — GetDatasets. No mutations.
- `engagement_EngagementService__*` — RecordEngagement.

Grep a method: `ls api-catalog | grep <Service>__<Method>`.
