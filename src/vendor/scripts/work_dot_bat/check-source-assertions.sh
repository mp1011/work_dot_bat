#!/usr/bin/env bash
# Thin wrapper around scripts/work_dot_bat/typescript/check-source-assertions.mjs (Node; only for TypeScript projects). See that file and
# agent_docs/plans/august_2026/unfalsifiable_test_gate.md for the design.
#
#   scripts/work_dot_bat/check-source-assertions.sh check      # fail if any test file has a flagged source-text assertion
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec node "$here/typescript/check-source-assertions.mjs" "$@"
