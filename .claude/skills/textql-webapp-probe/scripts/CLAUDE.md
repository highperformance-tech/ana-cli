# scripts

Helpers for the probe skill. Two-stage redaction pipeline: `normalize_request.sh` scrubs **secrets**, `anonymize_catalog.py` scrubs **PII / unique IDs**. Both must run before a catalog entry is committed; the project `pre-commit` hook enforces the anonymizer via `--check`.

## Files

- `normalize_request.sh` — bash/jq. Strips auth headers and redacts sensitive body keys (`apiKeyHash`, `password`, `*Token`, `*Secret`, `privateKey`, …), then reshapes captures into the catalog entry format. Add new sensitive key names to the `sensitive_names` list at the top.
- `anonymize_catalog.py` — python3. Scrubs identity data via pattern passes (UUIDs, emails, Slack IDs, signed URLs, Databricks workspace hosts, identity tokens) plus key-aware redaction (always-collapse keys, length-gated keys, name-mapping with Slack-sibling escape, int-ID demotion). Walks embedded-JSON string values with the same rules. Deterministic + idempotent; `--check` returns non-zero if any file would change. Extend the in-script tables when a new identifier shape appears; never hand-edit catalog entries.
- `identity_tokens.local.py` — **gitignored**. Real-name → placeholder table loaded by the anonymizer at import time. Patterns themselves are PII; never commit.
- `diff_catalog.sh` — diffs a fresh capture against the existing catalog entry for the same endpoint to spot shape drift.
