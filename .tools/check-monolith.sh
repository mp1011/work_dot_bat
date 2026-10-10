#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/check-monolith.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: 964494fbda93e3202619435c4a86c3949ddcab70cd04b6584d27e9b81732b019
# Thin wrapper around the `work_dot_bat check-monolith` command. See that command and
# agent_docs/plans/july_2026/monolith_detection.md for the design.
#
#   scripts/work_dot_bat/check-monolith.sh check      # fail if any file is a new/grown monolith
#   scripts/work_dot_bat/check-monolith.sh baseline   # regenerate the baseline from current disk state
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
exec "$bin" ${runflag:-} check-monolith "$@"
