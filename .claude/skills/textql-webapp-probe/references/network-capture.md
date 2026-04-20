# Network capture reference

## What `browser_network_requests` returns

An array of request records. Each record typically contains:

- `url` — full URL, including query string
- `method` — HTTP verb
- `status` — response status code (if response arrived)
- `requestHeaders` — object of header name → value
- `requestBody` — string or object, may be absent
- `responseHeaders` — object
- `responseBody` — string (may be JSON-encoded text) or absent for large/binary bodies
- `resourceType` — e.g. `xhr`, `fetch`, `document`, `script`, `image`, `font`, `stylesheet`

The exact shape can drift between Playwright MCP versions. When in doubt, ask `browser_evaluate` to `JSON.stringify` a sample, or inspect the first record's keys before writing filters that assume a field exists.

## Host filter

Keep only requests whose URL host matches one of:

- `api.textql.com`
- `app.textql.com` (keep only if `resourceType` is `xhr` or `fetch` — the HTML shell is not interesting)
- any other `*.textql.com` host serving JSON

Drop everything else. Common noise to drop explicitly: `*.segment.io`, `*.sentry.io`, `*.intercom.io`, `*.fullstory.com`, `fonts.googleapis.com`, `fonts.gstatic.com`, `*.posthog.com`, `*.launchdarkly.com`, `*.amplitude.com`, Cloudflare challenge endpoints.

## Resource type filter

Within TextQL hosts, keep only `xhr` and `fetch`. Drop `document`, `script`, `stylesheet`, `image`, `font`, `media`, `other`.

## Fields to record

For each kept request, persist:

- `method`
- `url` (full)
- `path` (pathname only, with any UUID/numeric ID segments replaced by `:id` — see path-slug rule in `catalog-schema.md`)
- `queryParams` — object of name → **example** value. Flag which params look required vs optional across repeated probes.
- `requestHeaders` — object. **Remove** `authorization`, `cookie`, `set-cookie`, and any header whose name ends in `-token`, `-secret`, `-key`. Also remove `x-csrf-token` values (keep the key, blank the value).
- `requestBody` — JSON if parseable; otherwise the raw string. If >50KB, replace with the inferred schema (see below).
- `status`
- `responseBody` — JSON if parseable. If >50KB or binary, replace with the inferred schema.
- `inferredRequestSchema`, `inferredResponseSchema` — always record these in addition to the sample bodies. Schema format: a recursive object mapping field name to its JSON type (`"string" | "number" | "boolean" | "null" | "object" | "array<T>"`). Arrays: use the schema of the first element, note `length` separately if instructive. This is what the CLI code generator will consume.

## Redaction

Two stages, both required. Stage 1 kills secrets. Stage 2 kills PII / unique IDs. The project `pre-commit` hook enforces stage 2 via `anonymize_catalog.py --check`.

### Stage 1 — secrets (`scripts/normalize_request.sh`)

Run every record through this first. The script:

- Deletes `authorization`, `cookie`, `set-cookie` headers case-insensitively.
- Blanks values of headers ending in `-token`, `-secret`, `-key`.
- Walks request + response bodies recursively and replaces string values with `"<REDACTED>"` when the key name (case-insensitive, underscores ignored) is sensitive. Covered: `apiKeyHash`, `plaintextKey`, `password`, `secret`, `clientSecret`, `accessToken`, `refreshToken`, `sessionToken`, `bearerToken`, `authToken`, `privateKey`, `signingKey`, `apiSecret`, `oauthSecret`, `webhookSecret`, plus any key ending in those suffixes (`refreshBearerToken`, `fooPassword`, …). Exempted: `apiKeyShort` (display fingerprint), `tokenType` (metadata), `csrfToken`, `publicKey`.

The body scrub is a safety net, not a substitute for review. After normalizing, eyeball the output: look for any remaining high-entropy strings, base64 blobs, or JWT-shaped values (three dot-separated segments). If a capture returns credential material under a field name not on the list above, **extend the list in `scripts/normalize_request.sh`** before writing the catalog entry — don't just edit the one file.

### Stage 2 — PII and unique IDs (`scripts/anonymize_catalog.py`)

`normalize_request.sh` only handles secrets. Real captures still contain emails, member/org UUIDs, Slack user/channel/team IDs, signed asset URLs, customer names baked into channel slugs, and free-text business content inside playbook prompts or generated Python. All of that must be scrubbed before the file lands in `api-catalog/`.

Run on every new or changed catalog file:

```
python3 .claude/skills/textql-webapp-probe/scripts/anonymize_catalog.py api-catalog/<file>.json
```

Deterministic per run (same UUID → same placeholder across every file in one invocation), idempotent (re-running produces no diff). Rewrites:

- UUIDs → `00000000-0000-0000-0000-<seq>` (both values and dict keys, including inside `inferredResponseSchema` where schema inference captures data-shaped keys).
- Emails → `user-<seq>@example.com`.
- Slack IDs (`U…`, `C…`, `T…`, `D…`, `G…`, `W…`) → `<prefix>REDACTED<seq>`.
- Integer IDs ≥ 100 000 under `id` / `memberId` / `userId` / `ownerId` → small deterministic ints.
- Signed asset URLs (`keyId=…`, `signature=…`) → `https://example.com/redacted/asset`.
- Known identity tokens (real person/org/customer/third-party SaaS names in `IDENTITY_TOKENS`) → neutral placeholders. When a new identifying token appears, **extend `IDENTITY_TOKENS`** in the script — don't hand-edit catalog files.
- Free-text keys that carry customer prose (`prompt`, `code`, `renderedHtml`, `output`, plus `content` / `summary` / `toolSummary` / `description` over 80 chars) → `"<REDACTED>"`. The schema under `inferredResponseSchema` is preserved so the catalog still tells you what shape the endpoint returns.

### Commit gate

`.git/hooks/pre-commit` runs `anonymize_catalog.py --check` against every staged `api-catalog/*.json` and blocks the commit if any file would change. If it fires, run the script (without `--check`) on the listed files, review the diff, and re-stage. Never commit until a diff review shows no plaintext secrets, emails, real UUIDs, or known identity tokens remain.

## Inferring schema from a sample body

Quick recipe, runnable via `browser_evaluate` or locally with `jq`:

```
jq 'def schema:
  if type == "object" then with_entries(.value |= schema)
  elif type == "array" then (if length == 0 then "array<unknown>" else "array<\(.[0] | schema)>" end)
  else type end;
schema'
```

Feed the response body through this and store the result in `inferredResponseSchema`. Do the same for `requestBody` when present.
