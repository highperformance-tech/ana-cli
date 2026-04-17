# Probe workflow

The goal of every probe: isolate the network calls a single UI action produces, record their shapes, and update the feature inventory if anything new showed up.

## Tool-call sequence

All tool names are the Playwright MCP names (`mcp__plugin_playwright_playwright__browser_*`), shortened below to `browser_*` for readability.

1. `browser_navigate` → `https://app.textql.com`
2. `browser_snapshot` — check whether a login form is visible.
   - If yes: tell the user "log in in the Playwright browser, then reply `ok`", and wait. Do not type credentials.
   - If already authed: continue.
3. Navigate to the page whose feature you want to probe (e.g. workspace list, a specific agent, a chat thread). Use `browser_navigate` for a direct URL, or `browser_click` + `browser_snapshot` to traverse the UI.
4. **Baseline capture:** call `browser_network_requests`. Remember the length of the returned array as `N_before`.
5. **Act:** perform the minimum UI interaction that exercises the feature. One action per probe — e.g. click "Create workspace", submit a chat message, open a single agent. Smaller actions = cleaner diffs.
6. `browser_wait_for` the UI change (a new element appearing, a loader disappearing) so in-flight requests finish before you re-capture.
7. **Re-capture:** call `browser_network_requests` again. Take the slice from index `N_before` onwards — that's the request set caused by the action.
8. **Filter:** keep only requests whose URL host matches the TextQL API (`api.textql.com` or any `*.textql.com` host that served JSON). Drop analytics, Sentry, Segment, Intercom, fonts, images, and cross-origin assets.
9. **Normalize:** pipe each kept request JSON into `scripts/normalize_request.sh` to produce a catalog record.
10. **Write:** for each record, write/overwrite `api-catalog/<METHOD>_<path-slug>.json` at the project root. Use the path slug rule in `references/catalog-schema.md`.
11. **Feature update:** if the action revealed a new page, entity, or concept not already in `docs/features.md`, append it per the format in `references/catalog-schema.md` with today's date as "last verified".
12. **Surface log:** append a one-line entry to `references/known-surfaces.md` naming the page/flow probed and today's date.

## Re-verification mode

When re-probing an endpoint to check it still works:

- Skip step 11 if nothing new was found.
- After step 10, run `scripts/diff_catalog.sh <old-dir> api-catalog/` against a snapshot from the last known-good commit (copy the directory aside first). A non-empty diff is the signal to update CLI code.

## Rules

- One UI action per probe. If you need to probe ten flows, do ten probes — don't try to bulk-capture.
- Never log or write `Authorization`, `Cookie`, or `Set-Cookie` values. `normalize_request.sh` strips them, but also don't echo them in tool results or chat.
- If `browser_network_requests` returns bodies that are obviously gigantic (>50KB) or binary, record just the schema (keys + types), not the full body. `references/network-capture.md` covers how.
- Do not refresh the page between probes in the same session unless the user asks. Fresh page = fresh login on many TextQL flows.
