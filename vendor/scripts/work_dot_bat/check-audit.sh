#!/usr/bin/env bash
# Thin wrapper around the `work_dot_bat check-audit` command. See that command and
# agent_docs/plans/august_2026/npm_vulnerability.md.
#
#   scripts/work_dot_bat/check-audit.sh check      # fail if `npm audit` reports any high/critical severity vulnerability
set -euo pipefail
bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2
  exit 127
fi
exec "$bin" check-audit "$@"
