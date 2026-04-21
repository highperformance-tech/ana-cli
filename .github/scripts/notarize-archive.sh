#!/usr/bin/env bash
# Notarizes a macOS archive produced by GoReleaser. Invoked from the release
# workflow in a loop over dist/*darwin*.tar.gz after `goreleaser release`.
#
# A .tar.gz cannot be stapled — Gatekeeper verifies the already-notarized
# Mach-O inside on first run (online check). The archive itself only needs
# to be accepted by notarytool.
set -euo pipefail

archive="$1"

xcrun notarytool submit "$archive" \
  --apple-id    "$APPLE_ID" \
  --password    "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id     "$APPLE_TEAM_ID" \
  --wait
