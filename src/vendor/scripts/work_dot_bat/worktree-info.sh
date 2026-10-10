#!/usr/bin/env bash
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
