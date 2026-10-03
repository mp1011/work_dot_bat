#!/usr/bin/env bash
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
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" plan-claim "$@"
