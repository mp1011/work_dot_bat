#!/usr/bin/env bash
# Regenerate the machine-owned file listing in agent_docs/03_project_structure.md,
# preserving every human-written summary. See the `work_dot_bat update-project-structure` command.
#
#   scripts/work_dot_bat/update-project-structure.sh            # rewrite the doc in place
#   scripts/work_dot_bat/update-project-structure.sh --check    # report drift, exit non-zero if any
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" update-project-structure "$@"
