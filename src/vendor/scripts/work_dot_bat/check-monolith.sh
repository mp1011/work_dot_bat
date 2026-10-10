#!/usr/bin/env bash
# Thin wrapper around the `work_dot_bat check-monolith` command. See that command and
# agent_docs/plans/july_2026/monolith_detection.md for the design.
#
#   scripts/work_dot_bat/check-monolith.sh check      # fail if any file is a new/grown monolith
#   scripts/work_dot_bat/check-monolith.sh baseline   # regenerate the baseline from current disk state
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" check-monolith "$@"
