#!/usr/bin/env bash
# Compare two api-catalog/ directories and print added, removed, and
# schema-changed endpoints. Exit 0 always (diff is informational).
#
# Usage: diff_catalog.sh <old-dir> <new-dir>

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "diff_catalog.sh: jq is required" >&2
  exit 1
fi

old_dir="${1:-}"
new_dir="${2:-}"

if [[ -z "$old_dir" || -z "$new_dir" ]]; then
  echo "usage: diff_catalog.sh <old-dir> <new-dir>" >&2
  exit 2
fi
if [[ ! -d "$old_dir" || ! -d "$new_dir" ]]; then
  echo "both arguments must be directories" >&2
  exit 2
fi

list_files() { (cd "$1" && find . -maxdepth 1 -name '*.json' -print | sed 's|^\./||' | sort); }

old_files="$(list_files "$old_dir")"
new_files="$(list_files "$new_dir")"

added="$(comm -13 <(printf '%s\n' "$old_files") <(printf '%s\n' "$new_files") || true)"
removed="$(comm -23 <(printf '%s\n' "$old_files") <(printf '%s\n' "$new_files") || true)"
common="$(comm -12 <(printf '%s\n' "$old_files") <(printf '%s\n' "$new_files") || true)"

schema_of() {
  # Prints the latest sample's inferredResponseSchema, or "null" if missing.
  jq -c '(.samples[-1].inferredResponseSchema // null)' "$1"
}

req_schema_of() {
  jq -c '(.samples[-1].inferredRequestSchema // null)' "$1"
}

changed=""
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  o_res="$(schema_of "$old_dir/$f")"
  n_res="$(schema_of "$new_dir/$f")"
  o_req="$(req_schema_of "$old_dir/$f")"
  n_req="$(req_schema_of "$new_dir/$f")"
  if [[ "$o_res" != "$n_res" || "$o_req" != "$n_req" ]]; then
    changed+="$f"$'\n'
  fi
done <<< "$common"

print_section() {
  local title="$1" body="$2"
  printf '### %s\n' "$title"
  if [[ -z "${body//[[:space:]]/}" ]]; then
    printf '  (none)\n\n'
  else
    printf '%s\n' "$body" | awk 'NF' | sed 's/^/  - /'
    printf '\n'
  fi
}

print_section "Added"   "$added"
print_section "Removed" "$removed"
print_section "Schema-changed" "$changed"
