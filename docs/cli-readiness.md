# CLI readiness review

Last updated: 2026-04-17.

A pass over `docs/features.md` and `api-catalog/` to call out what is solid enough to code against, and what a CLI implementer still has to figure out. Companion to `features.md`; no new findings that aren't also reflected there.

## TL;DR

- **Auth is SOLVED.** `Authorization: Bearer <apiKeyHash>` where `apiKeyHash` is the one-time plaintext returned by `rbac.RBACService/CreateApiKey`. Verified via curl with no cookies. See `features.md#api-shape-global` → "CLI auth".
- **CRUD is solid** for: chats (create/send/stream/get/history/rename/bookmark/duplicate/delete/share), API keys (create/rotate/revoke/list), service accounts (create/delete/list), connectors (test/create/get/update/delete, Postgres dialect).
- **Readonly is solid** for: dashboards, playbooks, feed, notifications, ontology, context library, observability stats, audit log, packages, SCIM, Slack binding.
- **Not covered at all by the probe** (CLI will have to probe or ask): dashboard CRUD, playbook CRUD (schedule + active + runNow), dataset CRUD, ontology CRUD, context prompt CRUD, report CRUD, file/attachment uploads.

## What a CLI can build today, by surface

Confidence key: ✅ full CRUD verified · 🟡 partial / readonly verified · ❗ known gap that needs a follow-up probe or a question to the TextQL team.

| Surface | State | Gap |
| --- | --- | --- |
| Auth (API key) | ✅ | None for personal keys. Service-account keys: untested end-to-end — we created an SA and then deleted it before creating a key scoped to it. |
| Chats | ✅ | `CreateChat.paradigm` — only `universal` observed; unknown if other paradigms exist (SQL-only? notebook?). Tool-selection flags (`sqlEnabled`, `pythonEnabled`, `webSearchEnabled`) defaults undocumented — must inspect `CreateChat` sample. `UpdateChat` only verified to touch `summary`; other mutable fields unknown. |
| Connectors | 🟡 | Only Postgres dialect verified. Other dialects assumed to follow `{config: {connectorType: <ENUM>, name, <dialect>: {...}}}` but field names per dialect are unknown — UI forms are the source of truth. |
| Service accounts | 🟡 | Created + deleted. NOT verified: creating an API key *on* a service account (the kebab menu has "Create API Key" — would need to confirm whether that uses `CreateApiKey` with a `memberId` override or a different RPC). |
| Dashboards | 🟡 | List/get/spawn/health covered. Create/update/delete NOT probed. |
| Playbooks | 🟡 | Get/list/reports/lineage covered. Create/update/delete/run-now NOT probed. |
| Sharing | 🟡 | `CreateShare` verified for chats via `LINK_COPY`. Other primitives (dashboard, playbook, report) and channels (slack?) not verified but shape is probably uniform. |
| Ontology | 🟡 | Readonly only. |
| Context prompts | 🟡 | Readonly only. |
| Datasets | 🟡 | `GetDatasets` only. No mutations probed. |
| SCIM | 🟡 | List only. Out of CLI scope unless admin tooling. |
| Slack | 🟡 | Read-only lookups (installations/users/channels). Sending messages through TextQL not captured. |
| Packages | 🟡 | List only; what "install/uninstall" looks like is unknown. |
| Notifications | 🟡 | Streaming envelope not captured (`StreamNotifications`). |
| Feed | 🟡 | Same — `StreamFeed` not captured. |

## Enum catalog (incomplete but useful)

From observed requests/responses. A CLI should expose these as string literals until it has a generated .proto.

- **Chat paradigm type:** `TYPE_UNIVERSAL`. `paradigm.version` is an integer schema version for `paradigm.options` (observed `1`).
- **Chat methodology:** `METHODOLOGY_ADAPTIVE` (others unverified).
- **Chat tool flags (live inside `paradigm.options.universal`, not at top level):** `connectorIds:[int]`, `sqlEnabled`, `pythonEnabled`, `webSearchEnabled`, `playbookToolsEnabled`.
- **Engagement / share primitives:** `PRIMITIVE_TYPE_CHAT` (others assumed: `_DASHBOARD`, `_PLAYBOOK`, `_REPORT` — unverified).
- **Engagement events:** `ENGAGEMENT_TYPE_VIEW` (write/edit events exist, unverified).
- **Share channels:** `SHARE_CHANNEL_LINK_COPY` (Slack channel variant assumed).
- **API key scope:** `API_KEY_SCOPE_ALL`.
- **API key sort fields:** `API_KEY_SORT_FIELD_CREATED_AT`.
- **Sort direction:** `SORT_DIRECTION_DESC`, `SORT_DIRECTION_ASC` (ASC unverified but conventional).
- **API key status:** `API_KEY_STATUS_ACTIVE` (revoked/expired values exist, unverified names).
- **Connector types:** `POSTGRES`. Other dialects (`BIGQUERY`, `SNOWFLAKE`, `REDSHIFT`, `MYSQL`, `SQLSERVER`, `DATABRICKS`, `SUPABASE`, `MOTHERDUCK`, `TABLEAU`, `POWERBI`) assumed but NOT verified in capture — UI exposes them.
- **Cell lifecycle:** `LIFECYCLE_CREATING` → `LIFECYCLE_CREATED` → `LIFECYCLE_EXECUTING` → `LIFECYCLE_EXECUTED`.
- **Cell variants:** `mdCell`, `summaryCell`, `statusCell`, `pyCell`, `playbookEditorCell`, `sqlCell`.
- **Models:** request uses `MODEL_DEFAULT`; responses observed `MODEL_HAIKU_4_5`, `MODEL_SONNET_4_6`.
- **Dashboard health:** `HEALTH_STATUS_HEALTHY` (unhealthy variants unverified).
- **Audit actions:** `api_key.created`, `api_key.rotated`, `api_key.revoked`, `service_account.created`, `service_account.deleted` — snake_case (NOT protobuf enum style), likely a free-form string the server writes.
- **Audit authMethod:** `oathkeeper` (web UI; CLI API key likely `api_key` — unverified).
- **Playbook action:** `PLAYBOOK_ACTION_LIST` (others unverified).

## Known quirks a CLI must handle

1. **Field casing:** camelCase only. snake_case alternates hit `duplicate field` 400s. Do not try to be clever.
2. **Error envelope inconsistency:** RPC layer returns `{code, message}`; reverse-proxy (Oathkeeper) returns `{error: {code, status, message}}`. CLI must accept both.
3. **Mixed ID types:** member / org / chat / dashboard / playbook / ontology-subset / share = UUID; connector / member-numeric (see `GetMember.id`) = integer. `member.id` (integer) is distinct from `member.memberId` (UUID); use `memberId` for RPC calls.
4. **Update shape:** `UpdateConnector` needs `connectorId` at the top level next to `config`; shoving it into `config` → 500. Future `Update*` RPCs probably follow the same pattern.
5. **Create bypasses Test:** `CreateConnector` does NOT require `TestConnector` to pass. CLI should run Test first and surface the error; otherwise you ship connectors that can't talk to anything.
6. **`apiKeyHash` is not a hash:** it's the plaintext key. Save it. Log carefully — never echo it.
7. **Chat rename uses `summary`:** not `name`. Easy footgun.
8. **StreamChat framing:** `[flags:1B][length:4B BE][JSON]` repeated; trailer flag `0x02`. Connect-RPC server-streaming. Every frame is a **full cell snapshot**, not a delta — correlate by `cell.id` and keep the last one. Standard buf-connect client handles this; a hand-rolled HTTP client does not.
9. **`CreateChat` without `connectorIds`:** produces a chat that can't receive messages — `SendMessage` will be blocked client-side. Treat `paradigm.options.universal.connectorIds` as required for any useful CLI flow.
10. **CreateServiceAccount roles are immutable:** no rename, no role edit. Recreate.
11. **DeleteServiceAccount cascades:** revokes every API key owned by that SA. Confirm before deleting.

## Recommended CLI command shape (first cut)

```
ana auth login                 # interactive: opens browser, captures apiKeyHash, stores in keychain
ana auth whoami                # GetMember
ana auth keys create --name X --expires 90d --inherit-all-roles
ana auth keys list
ana auth keys rotate <id>
ana auth keys revoke <id>

ana connector list
ana connector create postgres password --name X --host ... --database ...    # dialect + auth-mode subtree; runs Test, then Create
ana connector test <id>
ana connector update <id> --name X
ana connector delete <id>

ana chat new --connector <id> [--model MODEL_DEFAULT] [--research]
ana chat send <chatId> "message"            # POST SendMessage + read StreamChat until LIFECYCLE_CREATED
ana chat history <chatId>
ana chat rename <chatId> "new name"
ana chat bookmark <chatId>   / unbookmark
ana chat duplicate <chatId>
ana chat delete <chatId>
ana chat share <chatId>                     # CreateShare, prints URL

ana playbook list / get <id> / reports <id>
ana dashboard list / get <id> / spawn <id> / health <id>
ana ontology list / get <id>

ana audit tail                              # poll ListAuditLogs
```

Anything beyond this (`ana dashboard create`, `ana playbook schedule`, etc.) needs a fresh probe — the RPCs exist but their request shapes are not in the catalog yet.

## Follow-up probes to run before shipping a CLI

Priority order:

1. **Service-account-scoped API key creation.** The `/settings#dev` SA kebab menu has "View Keys" and "Create API Key". Open it on an existing SA, capture the `CreateApiKey` call — does it take a `memberId`/`serviceAccountId` param or is there a separate RPC?
2. **Playbook CRUD.** `/playbooks/new` and the edit surface on `/playbooks/<id>`. Probe `CreatePlaybook`, `UpdatePlaybook`, `DeletePlaybook`, `RunPlaybook`/`TriggerPlaybook`.
3. **Dashboard CRUD.** `/dashboards` "New Dashboard" flow. Probe `CreateDashboard` + edit.
4. **Non-Postgres connector dialects.** Capture one BigQuery, one Snowflake, one MySQL so the config-oneof fields are known. The rest can probably be inferred.
5. **`CreateShare` over Slack.** Open the Share dialog with Slack destination picked.
6. **Streaming RPCs** (`StreamNotifications`, `StreamFeed`). Re-use the envelope-framing scanner.
7. **`UpdateChat` beyond `summary`.** Try setting `connectorIds`, `research`, `model` to see if any are mutable post-create.

## Catalog hygiene

- `api-catalog/` has ~85 entries. Names use `POST_rpc__public__textql_rpc_public_<service>_<Service>__<Method>.json`. Grep-friendly.
- Every entry has `inferredRequestSchema` + `inferredResponseSchema` — these are type names, not proto. Use them to generate typed clients (e.g. TypeScript).
- `notes[]` is where quirks are spelled out. Read these before coding an endpoint — several (UpdateConnector id location, apiKeyHash being plaintext, CreateConnector not requiring Test) are not obvious from the request/response shapes.
