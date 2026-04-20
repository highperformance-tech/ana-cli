# references

Long-form docs loaded on demand by `SKILL.md`.

## Files

- `workflow.md` — exact Playwright tool-call sequence (navigate → baseline → act → re-capture → diff).
- `network-capture.md` — which `browser_network_requests` fields to keep, how to filter, and the two-stage redaction pipeline (secrets via `normalize_request.sh`, PII/unique IDs via `anonymize_catalog.py`).
- `catalog-schema.md` — JSON shape for `api-catalog/<file>.json` entries and markdown conventions for `docs/features.md`.
- `known-surfaces.md` — append-only log of probed pages/flows with catalog entries touched. Check before re-probing.
