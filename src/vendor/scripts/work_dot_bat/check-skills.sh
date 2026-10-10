#!/usr/bin/env bash
# Thin wrapper around the `work_dot_bat check-skills` command. See that command and
# agent_docs/plans/august_2026/aug_06_health_followup/03_repair_agents_skill_tree_and_add_check.md.
#
#   scripts/work_dot_bat/check-skills.sh check      # fail if any skill has broken path, name mismatch, or cross-tree routing mismatch
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" check-skills "$@"
