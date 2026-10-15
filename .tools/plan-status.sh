#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/plan-status.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: 52f5b464fc08bfe329985f4db1ed9559d19128a46b757c391058788903f93da7
# Mutate a plan file's status frontmatter (and regenerate the plan_status.js
# shim it drives) so nobody hand-edits a plan file's frontmatter block or the
# generated shim. See scripts/work_dot_bat/`work_dot_bat plan-status`.
#
#   scripts/work_dot_bat/plan-status.sh add <path> [status]                        # write frontmatter onto a new plan file (default: Not Started)
#   scripts/work_dot_bat/plan-status.sh add-subtask <path> -parent <path to parent> [status] # write frontmatter onto a subtask plan file
#   scripts/work_dot_bat/plan-status.sh set <path> <status> [--human]              # change an existing plan file's status ("Skipped"/"Cancelled" need --human)
#   scripts/work_dot_bat/plan-status.sh set-verification <path> <manual|automated> # mark whether an automated test covers this plan's behavior
#   scripts/work_dot_bat/plan-status.sh list [status]         # print entries, optionally filtered
#   scripts/work_dot_bat/plan-status.sh list-tasks [--free]   # print open tasks, indexed by number, worktree-annotated
#                                                 #   --free: only tasks not already advanced in a worktree
#                                                 #   ends with an unnumbered "N parked suggestions" footer when any are parked
#   scripts/work_dot_bat/plan-status.sh list-changed-done [ref] # print changed plans marked Done or Ready for Review against ref (default origin/main)
#   scripts/work_dot_bat/plan-status.sh backfill-subtasks     # backfill missing subtask_of frontmatter in plan.md folders
#   scripts/work_dot_bat/plan-status.sh ensure-frontmatter    # automatically add Not Started frontmatter to any plan file lacking it
#   scripts/work_dot_bat/plan-status.sh check [--stale-days <days>] # verify every tracked plan file has valid status frontmatter (warns on stale blocked plans)
#   scripts/work_dot_bat/plan-status.sh regenerate            # rebuild plan_status.js (gitignored) from disk, e.g. after a fresh clone
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
exec "$bin" ${runflag:-} plan-status "$@"
