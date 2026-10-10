#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/list-tasks.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: cd65ed9f32173871a0cf3a19b0ebfc736b44f9826615f1ea5917720f7a3eb160
# Print open tasks (incomplete plans) in agent_docs/plans, indexed by number.
# See scripts/work_dot_bat/`work_dot_bat plan-status`.
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! command -v "$bin" >/dev/null 2>&1; then
  if [ -f "$here/work" ]; then
    bin="$here/work"; runflag="--run"
  else
    echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
    exit 127
  fi
fi
exec "$bin" ${runflag:-} list-tasks "$@"
