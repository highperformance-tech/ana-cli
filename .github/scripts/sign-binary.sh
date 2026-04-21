#!/usr/bin/env bash
# Signs a freshly-built unix binary. Invoked by GoReleaser as a builds.hooks.post
# on the `ana-unix` build so the signature is embedded before archiving.
#
#   darwin  -> codesign with Developer ID Application (keychain imported by
#              the release workflow). Archive is later notarized in
#              notarize-archive.sh.
#   linux   -> no-op. Linux distros don't consume Authenticode-style sigs;
#              the sha256 in checksums.txt is the integrity anchor.
#
# Windows binaries are signed on a separate Windows job with
# azure/trusted-signing-action before they ever reach goreleaser, so this
# script is only wired to the `ana-unix` build.
set -euo pipefail

path="$1" os="$2"

case "$os" in
  darwin)
    codesign --force --timestamp --options=runtime \
      --sign "Developer ID Application" "$path"
    codesign --verify --strict "$path"
    ;;
  linux)
    ;;
  *)
    echo "sign-binary.sh: unexpected os=$os" >&2
    exit 1
    ;;
esac
