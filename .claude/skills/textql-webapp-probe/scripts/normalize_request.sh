#!/usr/bin/env bash
# Read one request record from stdin (as emitted by browser_network_requests,
# possibly with extra fields), write a catalog-shaped record to stdout.
#
# Redacts Authorization / Cookie / Set-Cookie headers and blanks values of any
# header whose name ends in -token, -secret, or -key. Bodies are left intact —
# redact secret field values inside bodies manually.
#
# Usage: normalize_request.sh <path-template> [description]
#   cat request.json | normalize_request.sh /api/v1/workspaces/:id/agents "list agents"

set -euo pipefail

if ! command -v jq >/dev/null 2>&1; then
  echo "normalize_request.sh: jq is required" >&2
  exit 1
fi

path_template="${1:-}"
description="${2:-}"
today="$(date -u +%Y-%m-%d)"

if [[ -z "$path_template" ]]; then
  echo "usage: normalize_request.sh <path-template> [description]" >&2
  exit 2
fi

jq -n \
  --arg pathTemplate "$path_template" \
  --arg description "$description" \
  --arg today "$today" \
  --argjson req "$(cat)" '
def redact_headers:
  with_entries(
    (.key | ascii_downcase) as $k
    | if $k == "authorization" or $k == "cookie" or $k == "set-cookie" then empty
      elif ($k | endswith("-token")) or ($k | endswith("-secret")) or ($k | endswith("-key")) then
        .value = ""
      else . end
  );

def infer_schema:
  if type == "object" then with_entries(.value |= infer_schema)
  elif type == "array" then
    if length == 0 then "array<unknown>" else "array<\(.[0] | infer_schema)>" end
  else type end;

def parse_maybe_json:
  if type == "string" then (try fromjson catch .) else . end;

($req.url // "") as $url
| ($url | capture("^https?://(?<h>[^/?#]+)").h // "") as $host
| ($req.requestHeaders // {} | redact_headers) as $reqH
| ($req.requestBody // null | parse_maybe_json) as $reqB
| ($req.responseBody // null | parse_maybe_json) as $resB
| {
    method: ($req.method // "GET"),
    pathTemplate: $pathTemplate,
    host: $host,
    description: $description,
    lastVerified: $today,
    samples: [{
      capturedAt: $today,
      url: $url,
      queryParams: ($req.queryParams // {}),
      requestHeaders: $reqH,
      requestBody: $reqB,
      status: ($req.status // null),
      responseBody: $resB,
      inferredRequestSchema: ($reqB | if . == null then null else infer_schema end),
      inferredResponseSchema: ($resB | if . == null then null else infer_schema end)
    }],
    notes: []
  }
'
