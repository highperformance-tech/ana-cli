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

Run every record through `scripts/normalize_request.sh` before writing to disk. The script:

- Deletes `authorization`, `cookie`, `set-cookie` headers case-insensitively.
- Blanks values of headers ending in `-token`, `-secret`, `-key`.
- Leaves request/response bodies alone — redact field values inside the body manually if they are obviously secret (e.g. API keys in a response). When in doubt, keep the key but replace the value with `"<redacted>"`.

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
