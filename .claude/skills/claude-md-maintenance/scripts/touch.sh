#!/usr/bin/env bash
# Touches a CLAUDE.md file to mark it as current.
# Usage: bash touch.sh <path/to/CLAUDE.md>
# Exit code: 0 = success, 1 = invalid input

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: touch.sh <path/to/CLAUDE.md>" >&2
  exit 1
fi

target="$1"

if [[ "$(basename "$target")" != "CLAUDE.md" ]]; then
  echo "Error: target must be a CLAUDE.md file, got '$(basename "$target")'" >&2
  exit 1
fi

if [[ ! -f "$target" ]]; then
  echo "Error: '$target' does not exist" >&2
  exit 1
fi

touch "$target"
echo "Touched: $target"
