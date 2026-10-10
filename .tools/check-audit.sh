#!/usr/bin/env bash
# work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
# file: scripts/work_dot_bat/check-audit.sh
# version: 2.0.0
# date: 2026-10-03
# sha256: bdfeb0727d168d2194594a47b7afb39aca212afcac483b26d72dfdb5fd930b24
# Thin wrapper around the `work_dot_bat check-audit` command. See that command and
# agent_docs/plans/august_2026/npm_vulnerability.md.
#
#   scripts/work_dot_bat/check-audit.sh check      # fail if `npm audit` reports any high/critical severity vulnerability
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
exec "$bin" ${runflag:-} check-audit "$@"
