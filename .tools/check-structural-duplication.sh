#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/check-structural-duplication.sh
# version: 1.1.0
# date: 2026-10-03
# sha256: cffbdb8bd87c02184cdb27e34979a5efaa3d2d38ad1b2c03918bd9b8c14737d1
# Thin wrapper around scripts/work_dot_bat/typescript/check-structural-duplication.mjs (Node; only for TypeScript projects). See that file and
# agent_docs/plans/august_2026/structural_duplication_detection.md.
#
#   scripts/work_dot_bat/check-structural-duplication.sh check                      # fail if structural duplication exceeds baseline
#   scripts/work_dot_bat/check-structural-duplication.sh update [--allow-regression] # rewrite baseline to current count
#   scripts/work_dot_bat/check-structural-duplication.sh report [options]           # generate diagnostic report (HTML/Markdown)
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec node "$here/typescript/check-structural-duplication.mjs" "$@"
