#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/worktree-info.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: c947f91b489a62cdb97938e897824fd29b3081224b094aac13f197a92da5c7e2
# Read-only: prints the current worktree's path/branch, the main checkout's
# path, and whether they're the same. See the `work_dot_bat worktree-info` command.
#
#   scripts/work_dot_bat/worktree-info.sh
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" worktree-info "$@"
