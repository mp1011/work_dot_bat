#!/usr/bin/env bash
# Builds only the Windows amd64 binary that the bin launcher runs, for quick local testing.
#   scripts/build-local.sh
# Use scripts/build-release.sh for all six OS/arch targets.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
if ! go build -o dist/work_dot_bat_windows_amd64.exe ./src/cmd/work_dot_bat; then
  echo "ERROR: build failed; dist/work_dot_bat_windows_amd64.exe was NOT updated." >&2
  echo "If the message above says the file is in use, quit every running work/work_dot_bat session and retry." >&2
  exit 1
fi
echo "built dist/work_dot_bat_windows_amd64.exe"
