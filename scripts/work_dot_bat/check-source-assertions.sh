#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/check-source-assertions.sh
# version: 1.1.0
# date: 2026-10-03
# sha256: 2b0e872ea62949fb085e0773d99a28e3bbbfb91d554f8e7a62b1b5e02dbd6ab0
# Thin wrapper around scripts/work_dot_bat/typescript/check-source-assertions.mjs (Node; only for TypeScript projects). See that file and
# agent_docs/plans/august_2026/unfalsifiable_test_gate.md for the design.
#
#   scripts/work_dot_bat/check-source-assertions.sh check      # fail if any test file has a flagged source-text assertion
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec node "$here/typescript/check-source-assertions.mjs" "$@"
