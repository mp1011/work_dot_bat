#!/usr/bin/env bash
# Regenerate the machine-owned file listing in agent_docs/02_project_structure.md,
# preserving every human-written summary. See the `work_dot_bat update-project-structure` command.
#
#   scripts/work_dot_bat/update-project-structure.sh            # rewrite the doc in place
#   scripts/work_dot_bat/update-project-structure.sh --check    # report drift, exit non-zero if any
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -n "$bin" ]; then
  :
elif command -v work_dot_bat >/dev/null 2>&1; then
  bin="work_dot_bat"
elif [ -f "$here/work" ]; then
  bin="$here/work"
  runflag="--run"
elif command -v work >/dev/null 2>&1; then
  bin="work"
  runflag="--run"
elif command -v work.bat >/dev/null 2>&1; then
  bin="work.bat"
  runflag="--run"
else
  echo "work binary not found: put work or work_dot_bat on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" ${runflag:-} update-project-structure "$@"

