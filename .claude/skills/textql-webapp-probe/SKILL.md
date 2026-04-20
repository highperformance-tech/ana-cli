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
7. **Anonymize PII and unique IDs** by running `scripts/anonymize_catalog.py <file>` against every new/changed catalog entry. This is required before the file is committed — the project's `pre-commit` hook calls the same script in `--check` mode and will refuse commits that still contain raw emails, UUIDs, Slack IDs, known org/client tokens, or signed asset URLs. See **Anonymization** below.
8. If the UI exposed a page, entity, or concept not already listed, append a section to `docs/features.md` with today's date.
9. Append the page/flow you just probed to `references/known-surfaces.md` so the next run knows it's covered.

Full tool-call sequence and diffing pattern: see `references/workflow.md`.

## Authentication

Open `https://app.textql.com`. If a login screen is shown, **pause and ask the user to complete login in the Playwright-controlled browser window.** Do not scrape, guess, or type credentials. Once logged in, reuse the same browser session for every probe in the turn — do not re-navigate to the root page between probes unless the user asks.

## Where findings go

- `api-catalog/<METHOD>_<path-slug>.json` — one file per endpoint, schema in `references/catalog-schema.md`. Checked into git. Create the directory on first write.
- `docs/features.md` — human-readable inventory of TextQL features and domain entities, with "last verified" dates. Format in `references/catalog-schema.md`. Create on first write.

Before writing anything to disk, run it through `scripts/normalize_request.sh`. It strips `Authorization` / `Cookie` / `Set-Cookie` headers AND recursively redacts string values under sensitive body keys (`apiKeyHash`, `password`, `*Token`, `*Secret`, `privateKey`, etc. — full list in `references/network-capture.md`). **Never** commit raw captures. After normalizing, eyeball the output for any remaining high-entropy strings or JWT-shaped values before writing to `api-catalog/`; if a new sensitive key name shows up, extend the script's list rather than editing a single file.

## Anonymization

`normalize_request.sh` only handles secrets. Captured bodies still contain PII and unique identifiers — real emails, member/org UUIDs, Slack user/channel/team IDs, signed asset URLs, customer names embedded in channel slugs, and free-text business content inside playbook prompts or generated Python. **All of these must be anonymized before the catalog entry lands on disk.**

After `normalize_request.sh` has shaped a file, run:

```
python3 .claude/skills/textql-webapp-probe/scripts/anonymize_catalog.py api-catalog/<file>.json
```

The script is deterministic per run (same UUID → same placeholder across every file in one invocation) and idempotent (re-running produces no diff). It rewrites:

- UUIDs → `00000000-0000-0000-0000-<seq>` (in both values and dict keys, including inside `inferredResponseSchema` when schema inference captured data-shaped keys).
- Emails → `user-<seq>@example.com`.
- Slack IDs (`U…`, `C…`, `T…`, `D…`, `G…`, `W…`) → `<prefix>REDACTED<seq>`.
- Integer IDs ≥ 100 000 under `id` / `memberId` / `userId` / `ownerId` → small deterministic ints.
- Signed asset URLs (`keyId=…`, `signature=…`) → `https://example.com/redacted/asset`.
- Known identity tokens (real person names, the org name and its variants, customer names baked into channel slugs, third-party SaaS hostnames) → neutral placeholders. When a new identifying token shows up that the script doesn't already know about, **extend `IDENTITY_TOKENS` in `scripts/anonymize_catalog.py`** — do not hand-edit individual catalog files.
- Free-text keys that routinely carry customer prose (`prompt`, `code`, `renderedHtml`, `output`, plus `content` / `summary` / `toolSummary` / `description` over 80 chars) → `"<REDACTED>"`. The schema under `inferredResponseSchema` is preserved, so the catalog still tells you what shape the endpoint returns.

Gate: the project's `.git/hooks/pre-commit` runs `anonymize_catalog.py --check` against every staged `api-catalog/*.json` and blocks the commit if any file would change. If the hook fires, run the script (without `--check`) on the listed files, review the diff, and re-stage.

## Pointers

- `references/workflow.md` — exact tool-call sequence and the baseline → act → re-capture → diff pattern. Read before your first probe in a session.
- `references/network-capture.md` — how to filter `browser_network_requests` output, which fields to keep, and how to redact secrets and PII. Read whenever you're about to record a request.
- `references/catalog-schema.md` — JSON shape for catalog entries and the markdown conventions for `docs/features.md`. Read when writing output.
- `references/known-surfaces.md` — living list of pages/flows already probed. Read before starting so you don't re-probe something, and append to it after a successful probe.
- `scripts/anonymize_catalog.py` — PII / unique-ID scrubber. Required on every catalog file before commit; enforced by `.git/hooks/pre-commit`.
