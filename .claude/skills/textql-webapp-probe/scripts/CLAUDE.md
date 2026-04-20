# scripts

Helpers for the probe skill. Two-stage redaction pipeline: `normalize_request.sh` scrubs **secrets**, `anonymize_catalog.py` scrubs **PII / unique IDs**. Both must run before a catalog entry is committed; the project `pre-commit` hook enforces the anonymizer via `--check`.

## Files

- `normalize_request.sh` — bash/jq. Strips `Authorization`/`Cookie`/`Set-Cookie` headers and recursively redacts string values under sensitive body keys (`apiKeyHash`, `password`, `*Token`, `*Secret`, `privateKey`, …), then reshapes a raw capture into the catalog entry format. Always pipe captures through this first. If a new sensitive key name appears, extend the `sensitive_names` list in the script.
- `anonymize_catalog.py` — python3. Runs on an already-normalized catalog file to scrub identity data. Substitutions:
  - **Pattern passes (every string):** UUIDs → `00000000-0000-0000-0000-<seq>` (keys and values, incl. inside `inferredResponseSchema`); emails → `user-<seq>@example.com`; Slack IDs (`U…/C…/T…/D…/G…/W…`) → `<prefix>REDACTED<seq>`; signed asset URLs (`keyId=`/`signature=`) → `https://example.com/redacted/asset`; Databricks warehouse IDs (`/sql/1.0/warehouses/<hex>`) → `.../REDACTED`; Databricks workspace hosts (`dbc-…cloud.databricks.com`) → `dbc-workspace.cloud.databricks.com`; known identity tokens loaded from gitignored `identity_tokens.local.py`.
  - **Key-aware redaction:**
    - Always-collapse keys → `"<REDACTED>"`: `prompt`, `code`, `renderedHtml`, `htmlPreview`, `subject`, `imageAlt`, `summary`, `content_preview`, `realName`, `organizationName`, `organization_name`, `organization_slug`, `agentName`, `patName`, `siteName`, `projectName`, `workbookName`, `shareToken`, `apiKeyShort`.
    - Length-gated (>80 chars) → `"<REDACTED>"`: `content`, `toolSummary`, `description`.
    - `heading` → `"<REDACTED>"` unless the value is a hex color (theme objects).
    - `output`/`items` list[str] → `["<REDACTED>"]`; `roles`/`activeAgentNames`/`connectorNames` string elements → per-item `"<REDACTED>"`.
    - `name`/`fullName`/`firstName`/`lastName`/`displayName`: filenames → `file-<seq>.<ext>` (extension preserved); kebab/snake slugs → `slug-<seq>`; multi-token strings → `Example Name <seq>`. Dicts containing a Slack sibling ID (`channelId`/`teamId`/`slackUserId`/`userId`) force bare-word `name` values through the slug map so single-token channel/user names (`general`, `me1`, `_tc25`) can't leak.
    - `email`/`emailAddress`/`ownerEmail` → email mapper; `profileImageUrl`/`avatarUrl`/`imageUrl` → `https://example.com/avatar.png`.
    - Int IDs ≥ 100_000 under `id`/`memberId`/`userId`/`ownerId` → deterministic small seq.
  - **Embedded-JSON recursion:** string values that parse as JSON (e.g. `orgMeta`, `responseBody`) are walked with the same key-aware rules so nested customer data gets the full treatment.
  - Deterministic per run, idempotent, `--check` mode returns non-zero if any file would change. When a new identifying token appears, extend `IDENTITY_TOKENS` in `identity_tokens.local.py` rather than hand-editing catalog entries.
- `identity_tokens.local.py` — **gitignored**. Holds the real-name → placeholder table (`IDENTITY_TOKENS: list[tuple[re.Pattern[str], str]]`). The committed anonymizer loads this at import time via `importlib.util`; `re`, `_START`, and `_END` are injected as globals. Never commit this file — the patterns themselves are PII.
- `diff_catalog.sh` — diffs a fresh capture against the existing catalog entry for the same endpoint, so you can tell when shape has drifted.
