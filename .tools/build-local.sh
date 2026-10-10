#!/usr/bin/env bash
# Builds only the Windows amd64 binary that the bin launcher runs, for quick local testing.
#   scripts/build-local.sh
# Use scripts/build-release.sh for all six OS/arch targets.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
go build -o dist/work_dot_bat_windows_amd64.exe ./src/cmd/work_dot_bat
echo "built dist/work_dot_bat_windows_amd64.exe"
