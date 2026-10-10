#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/check-skills.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: 9476d4e938c7ed325a32b8077804187ecb9932ec4691b3822dbfe89bdcea8819
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
