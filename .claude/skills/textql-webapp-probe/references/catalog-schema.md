# Catalog + features.md schema

## `api-catalog/` layout

One file per distinct `(method, path-template)` pair.

**Path-template rule:** take the URL pathname, and replace each segment that is a UUID, ULID, or pure-numeric ID with `:id`. If two ID-shaped segments appear in the same path, number them `:id1`, `:id2`, left to right. Examples:

- `/api/v1/workspaces/8f3b…/agents` → `/api/v1/workspaces/:id/agents`
- `/api/v1/threads/42/messages/7` → `/api/v1/threads/:id1/messages/:id2`

**Filename rule:** `<METHOD>_<slug>.json` where `<slug>` is the path-template with leading `/` dropped, `/` replaced by `__`, and `:` dropped. Example:

- `GET /api/v1/workspaces/:id/agents` → `api-catalog/GET_api__v1__workspaces__id__agents.json`

## Catalog entry JSON shape

Two variants. Default is the **sample-bearing** variant (body below). The **shape-only** variant skips the raw capture and keeps just the inferred schemas + notes — use it when the captured body carries business content the anonymizer can't safely scrub (customer prose, unique IDs that tie back to real environments, secrets whose shape we care about but whose values we cannot retain).

### Sample-bearing variant (default)

```json
{
  "method": "GET",
  "pathTemplate": "/api/v1/workspaces/:id/agents",
  "host": "api.textql.com",
  "description": "Short one-liner — what the UI was doing when this fired.",
  "lastVerified": "YYYY-MM-DD",
  "samples": [
    {
      "capturedAt": "YYYY-MM-DD",
      "url": "https://api.textql.com/api/v1/workspaces/abc/agents?limit=20",
      "queryParams": { "limit": "20" },
      "requestHeaders": { "content-type": "application/json", "x-csrf-token": "" },
      "requestBody": null,
      "status": 200,
      "responseBody": { "items": [ { "id": "…", "name": "…" } ], "nextCursor": null },
      "inferredRequestSchema": null,
      "inferredResponseSchema": {
        "items": "array<{ id: string, name: string }>",
        "nextCursor": "string"
      }
    }
  ],
  "notes": [
    "Free-form observations. E.g. 'Returns 401 with body {error:\"…\"} when CSRF missing.'"
  ]
}
```

### Shape-only variant

Top-level `inferredRequestSchema` + `inferredResponseSchema`, empty `samples`, scrubbed `notes`. Schema values are TypeScript-ish strings (`"string"`, `"integer"`, `"array<…>"`) — not literal example values. Used for connector-create flows where the request body is a live credential or workspace identifier.

```json
{
  "method": "POST",
  "pathTemplate": "/rpc/public/textql.rpc.public.connector.ConnectorService/CreateConnector",
  "host": "app.textql.com",
  "description": "Short one-liner.",
  "lastVerified": "YYYY-MM-DD",
  "samples": [],
  "inferredRequestSchema": { "config": { "name": "string", "…": "…" } },
  "inferredResponseSchema": { "connectorId": "integer", "name": "string" },
  "notes": [
    "Observations about the shape, wire/UI label mismatches, auth-mode discrimination, server-side defaults, …"
  ]
}
```

### Update rules

- Sample-bearing first capture: create the file with one entry in `samples`.
- Sample-bearing re-probe: if `inferredResponseSchema` matches the latest sample, overwrite `lastVerified` and replace `samples` with the new single capture.
- Shape-only first capture: keep `samples` empty; fill the top-level `inferredRequestSchema` + `inferredResponseSchema` from normalized observation. Pile observations into `notes` — they are the only persistent record.
- Shape-only re-probe: overwrite `lastVerified`; update schemas/notes if the wire shape moved.
- Schema drift (either variant): append a note dated the drift date and update the schemas. This is the signal the CLI needs attention.
- Never delete a catalog file manually — if an endpoint disappears, leave the file and add a note with the removal date.

## `docs/features.md` format

Top of file:

```markdown
# TextQL features & domain entities

Human-readable inventory, maintained by the `textql-webapp-probe` skill. Last updated: YYYY-MM-DD.
```

One `## ` section per feature area (e.g. `## Workspaces`, `## Agents`, `## Threads`, `## Integrations`). Each section:

```markdown
## Workspaces

- **Entity:** `Workspace` — top-level tenant container.
- **Pages:** list, detail, settings.
- **Related endpoints:** `GET /api/v1/workspaces`, `POST /api/v1/workspaces`, …
- **Notes:** anything non-obvious (permissions model, quirks).
- **Last verified:** YYYY-MM-DD
```

Keep it scannable. One bullet per fact. If a feature is deprecated or removed from the UI, leave the section and add a `- **Status:** removed YYYY-MM-DD` bullet.
