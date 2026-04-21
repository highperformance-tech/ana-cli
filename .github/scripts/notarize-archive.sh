#!/usr/bin/env bash
# Notarizes the macOS binary inside a GoReleaser tar.gz. Invoked from the
# release workflow in a loop over dist/*darwin*.tar.gz after goreleaser runs.
#
# notarytool only accepts .zip, .pkg, or .dmg, so we extract the binary,
# zip it, submit, and discard the wrapper. Notarization is registered
# against the Developer ID certificate identity — once Apple accepts a
# signed Mach-O for a given cert, Gatekeeper clears any later Mach-O
# signed with that same cert+timestamp on first-use online check. The
# tar.gz itself isn't stapleable, and stapling a CLI binary isn't
# meaningful; the online check is sufficient.
set -euo pipefail

archive="$1"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

tar -xzf "$archive" -C "$workdir" ana

zip_path="$workdir/$(basename "$archive" .tar.gz).zip"
(cd "$workdir" && zip -q "$zip_path" ana)

xcrun notarytool submit "$zip_path" \
  --apple-id    "$APPLE_ID" \
  --password    "$APPLE_APP_SPECIFIC_PASSWORD" \
  --team-id     "$APPLE_TEAM_ID" \
  --wait
