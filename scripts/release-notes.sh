#!/usr/bin/env bash
# Print the CHANGELOG.md section for a version, for `goreleaser release
# --release-notes`. Usage: scripts/release-notes.sh 1.7.2
set -euo pipefail
ver="${1:?usage: scripts/release-notes.sh <version>}"
notes="$(cd "$(dirname "$0")/.." && pwd)/CHANGELOG.md"
out="$(awk -v v="## ${ver}" '
  $0 == v { f = 1; next }
  f && /^## / { exit }
  f { print }
' "$notes")"
if [ -z "$(printf '%s' "$out" | tr -d '[:space:]')" ]; then
  echo "release-notes.sh: no section '## ${ver}' in ${notes}" >&2
  exit 1
fi
printf '%s\n' "$out"
