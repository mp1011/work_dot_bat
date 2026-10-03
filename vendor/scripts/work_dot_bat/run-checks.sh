#!/usr/bin/env bash
# Runs this repo's mechanical checks, build and test suites from work_dot_bat.config.json.
# See the `work_dot_bat run-checks` command. Used by the pre-push hook and CI.
#
#   scripts/work_dot_bat/run-checks.sh [--no-tighten] [--skip-tests] [--cwd <dir>]
#     --no-tighten   never rewrite baselines (CI)
#     --skip-tests   run only the mechanical checks and build commands
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" run-checks "$@"
