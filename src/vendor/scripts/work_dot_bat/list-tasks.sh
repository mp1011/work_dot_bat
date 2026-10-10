#!/usr/bin/env bash
# Print open tasks (incomplete plans) in agent_docs/plans, indexed by number.
# See scripts/work_dot_bat/`work_dot_bat plan-status`.
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" list-tasks "$@"
