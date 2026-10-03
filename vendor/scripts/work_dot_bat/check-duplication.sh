#!/usr/bin/env bash
# Thin wrapper around the `work_dot_bat check-duplication` command. See that command and
# agent_docs/plans/august_2026/aug_09_health_followup/08_duplication_ratchet_gate.md.
#
#   scripts/work_dot_bat/check-duplication.sh check                      # fail if duplication exceeds the baseline
#   scripts/work_dot_bat/check-duplication.sh update [--allow-regression] # rewrite the baseline to the current count
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" check-duplication "$@"
