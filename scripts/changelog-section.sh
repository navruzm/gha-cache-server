#!/usr/bin/env bash
# Print the CHANGELOG section for a given version.
# Usage: scripts/changelog-section.sh <version> [changelog-path]
# <version> is the bare semver, e.g. 0.2.0 (no leading "v").

set -euo pipefail

version="${1:?version required, e.g. 0.2.0}"
file="${2:-CHANGELOG.md}"

awk -v ver="$version" '
  $0 ~ "^## \\[" ver "\\]" { found = 1; next }
  found && /^## \[/        { exit }
  found                    { print }
' "$file"
