#!/usr/bin/env bash
# Signs a freshly-built binary. Invoked by GoReleaser as a builds.hooks.post
# so the signature is embedded before archiving.
#
#   darwin  -> codesign with Developer ID Application (keychain imported by
#              the release workflow). Archive is later notarized in
#              notarize-archive.sh.
#   windows -> Azure Trusted Signing via Microsoft's `sign` dotnet tool,
#              shared HPT "Elevate" profile under "tooling-and-automation".
#   linux   -> no-op. Linux distros don't consume Authenticode-style sigs;
#              the sha256 in checksums.txt is the integrity anchor.
set -euo pipefail

path="$1" os="$2"

case "$os" in
  darwin)
    codesign --force --timestamp --options=runtime \
      --sign "Developer ID Application" "$path"
    codesign --verify --strict "$path"
    ;;
  windows)
    sign code azure-trusted-signing \
      --azure-trusted-signing-endpoint "https://eus.codesigning.azure.net/" \
      --azure-trusted-signing-account "tooling-and-automation" \
      --azure-trusted-signing-certificate-profile "Elevate" \
      "$path"
    ;;
  linux)
    ;;
  *)
    echo "sign-binary.sh: unexpected os=$os" >&2
    exit 1
    ;;
esac
