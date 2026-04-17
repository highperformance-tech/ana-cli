#!/usr/bin/env bash
# Read one request record from stdin (as emitted by browser_network_requests,
# possibly with extra fields), write a catalog-shaped record to stdout.
#
# Redacts:
#   - Authorization / Cookie / Set-Cookie headers (deleted).
#   - Any header whose name ends in -token, -secret, or -key (value blanked).
#   - Any string value whose KEY name in request/response body matches a known
#     sensitive pattern (apiKeyHash, password, secret, *Token, *Secret, privateKey,
#     plaintextKey, clientSecret, etc.). Value replaced with "<REDACTED>".
#     Applied recursively inside arrays and nested objects.
#
# The body scrub is the safety net behind the human reviewer, not a replacement.
# When capturing a response that returns plaintext credential material, still
# review the output before committing.
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

# Sensitive body-field detection. Normalized key = lowercased, underscores
# stripped. Match is:
#   - exact name in the allowlist below, OR
#   - suffix match against {password, secret, privatekey, bearertoken, apikeyhash,
#     plaintextkey} so fields like "clientSecret" or "refreshBearerToken" are
#     also caught.
# Exemptions: keys lowercased to exactly "apikeyshort" (display-only), "tokentype",
# "csrftoken" (header value, already redacted elsewhere), "apikeyid" and
# "apikey" when their value is an object (metadata wrapper, not plaintext).
def sensitive_names: [
  "apikeyhash","plaintextkey","password","secret","clientsecret",
  "accesstoken","refreshtoken","sessiontoken","bearertoken","authtoken",
  "privatekey","signingkey","apisecret","oauthsecret","webhooksecret"
];

def sensitive_exempt: ["apikeyshort","tokentype","csrftoken","publickey"];

def is_sensitive_key($k):
  ($k | ascii_downcase | gsub("_";"")) as $lk
  | if (sensitive_exempt | any(. == $lk)) then false
    elif (sensitive_names | any(. == $lk)) then true
    else ($lk | test("(password|secret|privatekey|bearertoken|apikeyhash|plaintextkey)$"))
    end;

def redact_body:
  if type == "object" then
    with_entries(
      if (is_sensitive_key(.key)) and (.value | type == "string") and (.value | length > 0) then
        .value = "<REDACTED>"
      else
        .value |= redact_body
      end
    )
  elif type == "array" then map(redact_body)
  else . end;

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
| ($req.requestBody // null | parse_maybe_json | redact_body) as $reqB
| ($req.responseBody // null | parse_maybe_json | redact_body) as $resB
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
