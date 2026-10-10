#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/run-checks.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: 4720299a1a50f7408b14d55af3cec9c704e5019cb56f55655d1bf7d52859f38f
# Runs this repo's mechanical checks, build and test suites from work_dot_bat.config.json.
# See the `work_dot_bat run-checks` command. Used by the pre-push hook and CI.
#
#   scripts/work_dot_bat/run-checks.sh [--no-tighten] [--skip-tests] [--cwd <dir>]
#     --no-tighten   never rewrite baselines (CI)
#     --skip-tests   run only the mechanical checks and build commands
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! command -v "$bin" >/dev/null 2>&1; then
  if [ -f "$here/work_dot_bat" ]; then
    bin="$here/work_dot_bat"
  else
    echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
    exit 127
  fi
fi
exec "$bin" run-checks "$@"
