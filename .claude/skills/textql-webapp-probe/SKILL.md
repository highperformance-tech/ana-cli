---
name: textql-webapp-probe
description: Drive the TextQL web app (app.textql.com) in a Playwright-controlled browser to capture network requests, reverse-engineer undocumented API endpoints, and inventory UI features/domain entities. Use whenever building, extending, or repairing the ana Go CLI, verifying a previously-captured endpoint still works, or checking whether TextQL has shipped new pages/features/concepts the CLI should account for.
---

# textql-webapp-probe

TextQL ships its web app fast and its HTTP API is largely undocumented. Anything `ana-cli` knows about the API was learned by watching the browser. Use this skill to (re)learn it whenever the CLI needs to be built, extended, or repaired — or when you need to tell whether the app grew a new feature the CLI doesn't model yet.

## When to use

- Building a new `ana-cli` subcommand that calls a TextQL endpoint the CLI hasn't touched before.
- A CLI command started failing and you need to confirm whether the upstream endpoint changed shape, path, or status codes.
- Auditing the web app for new pages / domain entities / concepts that should be represented in the CLI.

If none of the above apply, stop — this skill has real-world side effects (opens a browser, expects the user to log in).

## Prerequisites

- Playwright MCP tools available: `mcp__plugin_playwright_playwright__browser_*` (especially `browser_navigate`, `browser_snapshot`, `browser_click`, `browser_fill_form`, `browser_evaluate`, `browser_network_requests`).
- `jq` on PATH (scripts depend on it).
- User has a working TextQL account. **Never** put credentials in the repo or in tool arguments.

## Workflow (high level)

1. Navigate to `https://app.textql.com` with `browser_navigate`.
2. Authenticate — see **Authentication** below.
3. Baseline: call `browser_network_requests` and note the current request count.
4. Drive the UI: click / fill / navigate to trigger the feature you are probing.
5. Re-capture with `browser_network_requests`; diff against the baseline to isolate new calls.
6. Normalize each relevant request through `scripts/normalize_request.sh` and write it to `api-catalog/<METHOD>_<path-slug>.json` at the project root.
7. If the UI exposed a page, entity, or concept not already listed, append a section to `docs/features.md` with today's date.
8. Append the page/flow you just probed to `references/known-surfaces.md` so the next run knows it's covered.

Full tool-call sequence and diffing pattern: see `references/workflow.md`.

## Authentication

Open `https://app.textql.com`. If a login screen is shown, **pause and ask the user to complete login in the Playwright-controlled browser window.** Do not scrape, guess, or type credentials. Once logged in, reuse the same browser session for every probe in the turn — do not re-navigate to the root page between probes unless the user asks.

## Where findings go

- `api-catalog/<METHOD>_<path-slug>.json` — one file per endpoint, schema in `references/catalog-schema.md`. Checked into git. Create the directory on first write.
- `docs/features.md` — human-readable inventory of TextQL features and domain entities, with "last verified" dates. Format in `references/catalog-schema.md`. Create on first write.

Before writing anything to disk, run it through `scripts/normalize_request.sh`, which strips `Authorization`, `Cookie`, and `Set-Cookie` headers. **Never** commit raw captures.

## Pointers

- `references/workflow.md` — exact tool-call sequence and the baseline → act → re-capture → diff pattern. Read before your first probe in a session.
- `references/network-capture.md` — how to filter `browser_network_requests` output, which fields to keep, and how to redact secrets. Read whenever you're about to record a request.
- `references/catalog-schema.md` — JSON shape for catalog entries and the markdown conventions for `docs/features.md`. Read when writing output.
- `references/known-surfaces.md` — living list of pages/flows already probed. Read before starting so you don't re-probe something, and append to it after a successful probe.
