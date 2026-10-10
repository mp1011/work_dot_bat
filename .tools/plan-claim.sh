#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/plan-claim.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: 2b0b083769e617ac92db07308de2caf7bd05ba9a698b5a9ca1b401842c1fd431
# The live-work claim registry CLI — "is anyone working on this right now?"
# See the `work_dot_bat plan-claim` command.
#
#   scripts/work_dot_bat/plan-claim.sh check <plan>                                   # read-only claim gate; exit 0 = free, 1 = held
#   scripts/work_dot_bat/plan-claim.sh concurrent-check <plan>                        # read-only Step Zero gate: claim + worktree-branch in one verdict; exit 0 = FREE, 1 = BLOCKED
#   scripts/work_dot_bat/plan-claim.sh acquire <plan> --owner <label> [--worktree <path>] [--steal] [--kind daemon|agent]
#   scripts/work_dot_bat/plan-claim.sh renew <plan>
#   scripts/work_dot_bat/plan-claim.sh release <plan>
#   scripts/work_dot_bat/plan-claim.sh list                                           # every live claim, with age and staleness
#   scripts/work_dot_bat/plan-claim.sh prune                                          # remove every stale-reclaimable claim (the launchers run this at startup)
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
exec "$bin" ${runflag:-} plan-claim "$@"
