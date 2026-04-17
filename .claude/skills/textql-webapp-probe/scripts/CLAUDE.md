# scripts

Bash helpers for the probe skill.

## Files

- `normalize_request.sh` — strips `Authorization`/`Cookie`/`Set-Cookie` headers and reshapes a raw capture into the catalog entry format. Always pipe captures through this before writing to `api-catalog/`.
- `diff_catalog.sh` — diffs a fresh capture against the existing catalog entry for the same endpoint, so you can tell when shape has drifted.
