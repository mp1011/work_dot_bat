#!/usr/bin/env bash
# Thin wrapper around scripts/work_dot_bat/typescript/check-structural-duplication.mjs (Node; only for TypeScript projects). See that file and
# agent_docs/plans/august_2026/structural_duplication_detection.md.
#
#   scripts/work_dot_bat/check-structural-duplication.sh check                      # fail if structural duplication exceeds baseline
#   scripts/work_dot_bat/check-structural-duplication.sh update [--allow-regression] # rewrite baseline to current count
#   scripts/work_dot_bat/check-structural-duplication.sh report [options]           # generate diagnostic report (HTML/Markdown)
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec node "$here/typescript/check-structural-duplication.mjs" "$@"
