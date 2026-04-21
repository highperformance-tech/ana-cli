#!/usr/bin/env python3
"""Anonymize PII and unique identifiers in api-catalog/*.json files.

Safety net that runs over a whole catalog or a single file. Intended uses:
  - After a probe session, before committing new / changed entries.
  - As a one-off sweep to scrub historical PII from already-checked-in entries.

Deterministic: the same UUID/email/Slack ID maps to the same placeholder across
every file in one run, so cross-file references stay consistent and diffs stay
reviewable. The map is rebuilt each run (no cross-run state), so seq numbers
are stable only within a single invocation.

Scope of changes:

1) Pattern-based scrub on every string value:
     - RFC4122 UUIDs      -> 00000000-0000-0000-0000-<seq>
     - Emails             -> user-<seq>@example.com
     - Slack IDs          -> <prefix>REDACTED<seq>  (U/C/T/D/G/W prefixes)
     - Signed asset URLs  -> https://example.com/redacted/asset
     - Databricks warehouse IDs (/sql/1.0/warehouses/<hex>) -> .../REDACTED
     - Databricks workspace hosts (dbc-*.cloud.databricks.com) -> generic
     - Known identity tokens (real names, org names) -> neutral placeholders
       (patterns loaded from gitignored identity_tokens.local.py)

2) Key-aware redaction for fields that routinely carry free-text customer
   data or identifying metadata. See CONTENT_KEYS_ALWAYS, CONTENT_KEYS_LONG,
   NAME_KEYS, EMAIL_KEYS, IMAGE_URL_KEYS, OUTPUT_LIST_KEYS,
   REDACT_STRING_LIST_KEYS for the current key sets. Highlights:
     - Free-text always-collapse keys (prompt/code/renderedHtml/htmlPreview/
       subject/imageAlt/summary/content_preview/realName/organizationName/
       organization_slug/agentName/patName/siteName/projectName/workbookName/
       shareToken/apiKeyShort) -> "<REDACTED>".
     - Length-gated free-text keys (content/toolSummary/description) ->
       "<REDACTED>" once longer than 80 chars.
     - `heading` -> "<REDACTED>" unless the value is a hex color.
     - `output`/`items` list[str] -> ["<REDACTED>"]; role/agent/connector
       name lists -> per-element "<REDACTED>".
     - name / fullName / firstName / lastName / displayName:
         * filenames -> file-<seq>.<ext>
         * kebab/snake slugs -> slug-<seq>
         * multi-token strings -> "Example Name <seq>"
         * dicts containing a Slack sibling ID (channelId/teamId/
           slackUserId/userId) force bare-word names (e.g. "general", "me1")
           through the slug map.
     - emailAddress / email / ownerEmail -> user-<seq>@example.com.
     - profileImageUrl / avatarUrl / imageUrl -> https://example.com/avatar.png.
     - Integer IDs >= INT_ID_MIN under id/memberId/userId/ownerId -> small
       deterministic sequence.

3) Embedded-JSON recursion: string values that parse as a JSON object / array
   (e.g. stringified `orgMeta`, `responseBody`) are walked with the full
   key-aware pipeline so nested customer data gets the same treatment as a
   top-level field, not a best-effort regex scrub of the wrapper string.

Every pass is idempotent: shapes the script itself emits are matched against
the ALREADY_* sentinels and left unchanged on re-runs.

Usage:
    scripts/anonymize_catalog.py [PATH ...]

If no PATH is given, defaults to every JSON under api-catalog/ at the repo
root. With --check the script exits non-zero when a file would change, which
lets a pre-commit hook block raw captures from landing.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import re
import sys
from pathlib import Path
from typing import Any

UUID_RE = re.compile(
    r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b", re.IGNORECASE
)
EMAIL_RE = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}")
# Slack IDs use a prefix letter + 8+ alphanumerics; require at least one digit
# in the tail so pure-letter enums like "DATABRICKS" / "WORKSPACE" don't get
# swept into the Slack mapper. Real Slack IDs have random hex-ish tails that
# always include digits in practice.
SLACK_RE = re.compile(r"\b(?P<prefix>U|C|T|D|G|W)(?=[A-Z0-9]*[0-9])[A-Z0-9]{8,}\b")
SIGNED_URL_RE = re.compile(r"https?://[^\s\"]+?(?:keyId|signature)=[^\s\"]+")
# Databricks warehouse IDs leak a stable internal cluster identifier. Preserve
# the `/sql/1.0/warehouses/` prefix so the endpoint shape is still readable.
DATABRICKS_HTTP_RE = re.compile(r"(/sql/1\.0/warehouses/)[0-9a-f]+", re.IGNORECASE)
# Databricks workspace hostnames embed a stable workspace ID. Collapse them to
# a generic placeholder that keeps the `.cloud.databricks.com` suffix.
DATABRICKS_HOST_RE = re.compile(
    r"\bdbc-[0-9a-f][0-9a-f-]+\.cloud\.databricks\.com\b", re.IGNORECASE
)

# Shapes already produced by a previous run — used to keep every sub
# idempotent so a re-run (or `--check`) against an already-clean file is a
# no-op. Without these guards the subs would remap "TREDACTED00042" to a new
# "TREDACTED00001", etc., which made the pre-commit hook trip on its own
# output.
ALREADY_UUID_RE = re.compile(r"^00000000-0000-0000-0000-\d{12}$")
ALREADY_EMAIL_RE = re.compile(r"^user-\d+@example\.com$")
ALREADY_SLACK_RE = re.compile(r"^[UCTDGW]REDACTED\d+$")
ALREADY_NAME_RE = re.compile(r"^Example Name \d+$")
ALREADY_FILE_RE = re.compile(r"^file-\d+\.[A-Za-z0-9]{1,8}$")
ALREADY_SLUG_RE = re.compile(r"^slug-\d+$")
# Fullmatch against shapes the anonymizer itself emits under name-like keys.
# Used so re-runs stay idempotent without relying on substring checks (which
# would skip e.g. "example-client-2 Films" and let the trailing word leak).
ALREADY_PLACEHOLDER_NAME_RE = re.compile(
    r"^(?:Example(?: [A-Z][A-Za-z0-9]*)+|example-[a-z][a-z0-9-]*|client-redacted|ext-redacted|ExampleOrg|ExampleSaaS|ExampleVideo)$"
)
HEX_COLOR_RE = re.compile(r"^#[0-9A-Fa-f]{3,8}$")
# Known file extensions we care about. Anything under a name-like key ending
# with one of these is treated as a real filename and rewritten to
# `file-<seq>.<ext>`.
FILENAME_RE = re.compile(
    r"^[A-Za-z0-9._ \-()]+\.(?:pdf|csv|xlsx|xls|txt|py|json|md|doc|docx|tsv|parquet|html|htm|zip|png|jpg|jpeg|gif|svg|yaml|yml|sql|ipynb)$",
    re.IGNORECASE,
)
# Kebab / snake slugs (at least one `-` or `_` separator). Deliberately
# excludes PascalCase / TitleCase product names ("Overview", "Customer",
# "Notion", "GitHub") — those are left alone.
SLUG_RE = re.compile(r"^[_a-z0-9][a-z0-9_]*[-_][a-z0-9][a-z0-9_\-]*$")

# Treat a token as ending whenever the next char is a non-letter/digit (so
# "_", ".", "-", "/", whitespace, end-of-string all qualify). Regular `\b`
# would miss "first_last_resume.pdf" because "_" is a word char.
_END = r"(?=[^A-Za-z0-9]|$)"
_START = r"(?<![A-Za-z0-9])"

# Identity tokens (real person / org / customer names and their placeholders)
# live in a sibling `identity_tokens.local.py` that is gitignored — the patterns
# themselves are PII we don't want in git history. The local file exposes an
# `IDENTITY_TOKENS` list and may use the injected `re`, `_START`, `_END`
# globals. If the file is absent the anonymizer still runs the generic regex
# passes (UUIDs, emails, Slack IDs, signed URLs) and the key-based redactions.
def _load_identity_tokens() -> list[tuple[re.Pattern[str], str]]:
    local = Path(__file__).resolve().parent / "identity_tokens.local.py"
    if not local.exists():
        return []
    spec = importlib.util.spec_from_file_location("_ana_identity_tokens", local)
    if spec is None or spec.loader is None:
        return []
    mod = importlib.util.module_from_spec(spec)
    mod.__dict__["re"] = re
    mod.__dict__["_START"] = _START
    mod.__dict__["_END"] = _END
    spec.loader.exec_module(mod)
    return list(getattr(mod, "IDENTITY_TOKENS", []))


IDENTITY_TOKENS: list[tuple[re.Pattern[str], str]] = _load_identity_tokens()

CONTENT_KEYS_ALWAYS = {
    "prompt",
    "code",
    "renderedHtml",
    "htmlPreview",
    # Playbook / feed report free-text. Short headings leak financials
    # ("OVERDUE — $14,300.00") so they are redacted unconditionally; the
    # `heading` key carries hex colors in theme objects and is handled
    # specially in _handle_value.
    "subject",
    "imageAlt",
    "summary",
    "content_preview",
    # Identity / account surface. These are names, slugs, and opaque tokens
    # the user asked to scrub categorically.
    "realName",
    "organizationName",
    "organization_name",
    "organization_slug",
    "agentName",
    # Tableau metadata — site / project / workbook / PAT names are all
    # identifying in a customer context.
    "patName",
    "siteName",
    "projectName",
    "workbookName",
    # API key / share surface.
    "shareToken",
    "apiKeyShort",
}
# Integer-valued keys that carry a unique record ID per entity. Values >=
# INT_ID_MIN get mapped to a small deterministic placeholder; small values are
# left alone because they tend to be demo/shared-example IDs (connector 70,
# 503, ...) whose leakage isn't meaningful.
INT_ID_KEYS = {"id", "memberId", "userId", "ownerId"}
INT_ID_MIN = 100_000
# Length-gated — short values are usually safe labels, long ones carry
# generated copy that may leak PII.
CONTENT_KEYS_LONG = {"content", "toolSummary", "description"}
# `heading` gets always-redacted EXCEPT when the value is a hex color (theme
# objects keep e.g. "#2D3748"). Handled in _handle_value.
HEADING_KEY = "heading"
NAME_KEYS = {"name", "fullName", "firstName", "lastName", "displayName"}
EMAIL_KEYS = {"email", "emailAddress", "ownerEmail"}
IMAGE_URL_KEYS = {"profileImageUrl", "avatarUrl", "imageUrl"}
OUTPUT_LIST_KEYS = {"output", "items"}
# String elements of lists under these keys are redacted individually. Covers
# member roles, feed agent names, and the connector-name summary list.
REDACT_STRING_LIST_KEYS = {"roles", "activeAgentNames", "connectorNames"}

# Keys whose "name" values are schema hints (e.g. inferredResponseSchema) or
# safe enums (connector types) — skip name redaction there.
SCHEMA_PARENT_KEYS = {"inferredRequestSchema", "inferredResponseSchema"}

# Sibling keys that, when present alongside `name` in a dict, mark that dict as
# a Slack channel / Slack user / connector metadata record. The `name` value is
# then forced to the slug map even if it would otherwise look like a safe bare
# word ("general", "sales", "vizstack"). Keeps single-token identifiers from
# leaking.
SLACK_NAME_SIBLING_KEYS = {"channelId", "teamId", "slackUserId", "userId"}


class Anonymizer:
    def __init__(self) -> None:
        self.uuid_map: dict[str, str] = {}
        self.email_map: dict[str, str] = {}
        self.slack_map: dict[str, str] = {}
        self.name_seq = 0
        self.file_map: dict[str, str] = {}
        self.slug_map: dict[str, str] = {}
        self.int_id_map: dict[int, int] = {}

    # --- pattern-based scrubbing ---------------------------------------------

    def _uuid_sub(self, m: re.Match[str]) -> str:
        raw = m.group(0)
        if ALREADY_UUID_RE.match(raw):
            return raw
        key = raw.lower()
        if key not in self.uuid_map:
            n = len(self.uuid_map) + 1
            self.uuid_map[key] = f"00000000-0000-0000-0000-{n:012d}"
        return self.uuid_map[key]

    def _email_sub(self, m: re.Match[str]) -> str:
        raw = m.group(0)
        if ALREADY_EMAIL_RE.match(raw):
            return raw
        key = raw.lower()
        if key not in self.email_map:
            n = len(self.email_map) + 1
            self.email_map[key] = f"user-{n}@example.com"
        return self.email_map[key]

    def _slack_sub(self, m: re.Match[str]) -> str:
        key = m.group(0)
        if ALREADY_SLACK_RE.match(key):
            return key
        if key not in self.slack_map:
            n = len(self.slack_map) + 1
            prefix = m.group("prefix")
            self.slack_map[key] = f"{prefix}REDACTED{n:05d}"
        return self.slack_map[key]

    def scrub_string(self, s: str) -> str:
        out = SIGNED_URL_RE.sub("https://example.com/redacted/asset", s)
        out = DATABRICKS_HTTP_RE.sub(r"\g<1>REDACTED", out)
        out = DATABRICKS_HOST_RE.sub("dbc-workspace.cloud.databricks.com", out)
        out = UUID_RE.sub(self._uuid_sub, out)
        out = EMAIL_RE.sub(self._email_sub, out)
        out = SLACK_RE.sub(self._slack_sub, out)
        for pattern, repl in IDENTITY_TOKENS:
            out = pattern.sub(repl, out)
        return out

    # --- key-aware walking ---------------------------------------------------

    def next_name(self) -> str:
        self.name_seq += 1
        return f"Example Name {self.name_seq}"

    def walk(self, node: Any, parent_key: str | None = None, in_schema: bool = False) -> Any:
        if isinstance(node, dict):
            # Slack channel / user records pair a sibling ID key with `name`.
            # Force bare-word names in that context to the slug map so tokens
            # like "general" / "me1" / "_tc25" / "vizstack" don't slip through.
            force_name_to_slug = not in_schema and any(
                k in SLACK_NAME_SIBLING_KEYS for k in node
            )
            new: dict[str, Any] = {}
            for k, v in node.items():
                child_in_schema = in_schema or k in SCHEMA_PARENT_KEYS
                # Dict keys can themselves be UUIDs (permission maps), emails,
                # or Slack IDs — in schema subtrees too, because schema
                # inference on a data-keyed map puts real IDs in the keys.
                # Scrub the key string always; keep the original key name in
                # the local binding for content-redaction decisions below.
                new_key = self.scrub_string(k) if isinstance(k, str) else k
                new[new_key] = self._handle_value(
                    k, v, child_in_schema, force_name_to_slug=force_name_to_slug
                )
            return new
        if isinstance(node, list):
            if parent_key in OUTPUT_LIST_KEYS and not in_schema:
                if any(isinstance(x, str) and x for x in node):
                    return ["<REDACTED>"]
            if parent_key in REDACT_STRING_LIST_KEYS and not in_schema:
                return [
                    "<REDACTED>" if isinstance(v, str) and v else self.walk(v, parent_key, in_schema)
                    for v in node
                ]
            return [self.walk(v, parent_key, in_schema) for v in node]
        if isinstance(node, str):
            # Some RPC bodies stringify their payload (e.g. `responseBody`,
            # `orgMeta`). Walk embedded JSON so nested `code` / `prompt` /
            # customer slugs get the same key-aware redaction as a top-level
            # field, not just a best-effort regex scrub of the raw string.
            recursed = self._scrub_embedded_json(node, parent_key, in_schema)
            if recursed is not None:
                return recursed
            return self.scrub_string(node)
        return node

    def _scrub_embedded_json(
        self, value: str, parent_key: str | None, in_schema: bool
    ) -> str | None:
        stripped = value.lstrip()
        if not stripped or stripped[0] not in "{[":
            return None
        try:
            parsed = json.loads(value)
        except (ValueError, TypeError):
            return None
        if not isinstance(parsed, (dict, list)):
            return None
        cleaned = self.walk(parsed, parent_key, in_schema)
        return json.dumps(cleaned, ensure_ascii=False)

    def _handle_value(
        self,
        key: str,
        value: Any,
        in_schema: bool,
        force_name_to_slug: bool = False,
    ) -> Any:
        # Schema subtrees store type strings like "string" / "array<...>", not
        # real values — leave them untouched.
        if in_schema:
            return self.walk(value, key, in_schema=True)

        if isinstance(value, bool):
            return value
        if isinstance(value, int) and key in INT_ID_KEYS and value >= INT_ID_MIN:
            if value not in self.int_id_map:
                self.int_id_map[value] = len(self.int_id_map) + 1
            return self.int_id_map[value]

        if isinstance(value, str) and value:
            if key in CONTENT_KEYS_ALWAYS:
                return "<REDACTED>"
            if key == HEADING_KEY:
                # Theme objects store hex colors under `heading`; everything
                # else is free-text report copy that we redact.
                if HEX_COLOR_RE.match(value):
                    return value
                return "<REDACTED>"
            if key in CONTENT_KEYS_LONG and len(value) > 80:
                return "<REDACTED>"
            recursed = self._scrub_embedded_json(value, key, in_schema)
            if recursed is not None:
                return recursed
            if key in EMAIL_KEYS:
                # Value is expected to BE an email; run it through the email
                # mapper so it shares sequencing with inline-email matches.
                return EMAIL_RE.sub(self._email_sub, value)
            if key in IMAGE_URL_KEYS:
                return "https://example.com/avatar.png"
            if key in NAME_KEYS:
                return self._handle_name(value, force_slug=force_name_to_slug)
            return self.scrub_string(value)

        return self.walk(value, key, in_schema)

    def _handle_name(self, value: str, force_slug: bool = False) -> str:
        # Hit the pattern passes first. If they rewrote anything, the value
        # already carries a placeholder — stop.
        scrubbed = self.scrub_string(value)
        if scrubbed != value:
            return scrubbed
        if ALREADY_NAME_RE.match(value):
            return value
        # Skip values that are *exactly* one of our placeholder shapes — this
        # keeps re-runs idempotent without letting compound values like
        # "example-client-2 Films" bypass the heuristic because they happen to
        # start with a placeholder token.
        if ALREADY_PLACEHOLDER_NAME_RE.match(value):
            return value
        if ALREADY_FILE_RE.match(value):
            return value
        if ALREADY_SLUG_RE.match(value):
            return value
        # Filename values (datasets[].name, etc.) — preserve the extension so
        # the shape is still useful; everything before the dot is redacted.
        if FILENAME_RE.match(value):
            ext = value.rsplit(".", 1)[1].lower()
            if value not in self.file_map:
                self.file_map[value] = f"file-{len(self.file_map) + 1}.{ext}"
            return self.file_map[value]
        # Kebab / snake slug values (Slack channels, internal site codes,
        # probe-test fixtures). Product / brand names like "Notion",
        # "GitHub" do not match — SLUG_RE requires a lowercase first letter.
        # `force_slug` pulls in single-token bare names too (Slack channel
        # `general`, user `me1`) when a sibling ID key disambiguated the dict.
        if SLUG_RE.match(value) or force_slug:
            if value not in self.slug_map:
                self.slug_map[value] = f"slug-{len(self.slug_map) + 1}"
            return self.slug_map[value]
        # Person-name heuristic: two-plus tokens separated by a space that
        # doesn't end with `)` (which would indicate an enum or parenthesised
        # label, not a human name).
        if " " in value and not value.endswith(")"):
            return self.next_name()
        return value


def process_file(path: Path, anon: Anonymizer, check: bool) -> bool:
    original = path.read_text()
    data = json.loads(original)
    cleaned = anon.walk(data)
    new_text = json.dumps(cleaned, indent=2, ensure_ascii=False) + "\n"
    changed = new_text != original
    if changed and not check:
        path.write_text(new_text)
    return changed


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", nargs="*", type=Path)
    parser.add_argument(
        "--check",
        action="store_true",
        help="Exit non-zero if any file would change; do not write.",
    )
    args = parser.parse_args(argv)

    if args.paths:
        targets: list[Path] = []
        for p in args.paths:
            if p.is_dir():
                targets.extend(sorted(p.glob("*.json")))
            else:
                targets.append(p)
    else:
        repo_root = Path(__file__).resolve().parents[4]
        targets = sorted((repo_root / "api-catalog").glob("*.json"))

    anon = Anonymizer()
    changed_files: list[Path] = []
    for p in targets:
        if process_file(p, anon, args.check):
            changed_files.append(p)

    if args.check:
        if changed_files:
            print("Files with PII still present:", file=sys.stderr)
            for p in changed_files:
                print(f"  {p}", file=sys.stderr)
            return 1
        return 0

    print(f"Scanned {len(targets)} file(s); rewrote {len(changed_files)}.")
    print(
        f"Mapped {len(anon.uuid_map)} UUIDs, {len(anon.email_map)} emails, "
        f"{len(anon.slack_map)} Slack IDs, {anon.name_seq} names, "
        f"{len(anon.file_map)} filenames, {len(anon.slug_map)} slugs."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
