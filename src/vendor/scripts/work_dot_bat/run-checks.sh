#!/usr/bin/env bash
# Runs this repo's mechanical checks, build and test suites from work_dot_bat.config.json.
# See the `work_dot_bat run-checks` command. Used by the pre-push hook and CI.
#
#   scripts/work_dot_bat/run-checks.sh [--no-tighten] [--skip-tests] [--cwd <dir>]
#     --no-tighten   never rewrite baselines (CI)
#     --skip-tests   run only the mechanical checks and build commands
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
exec "$bin" ${runflag:-} run-checks "$@"
