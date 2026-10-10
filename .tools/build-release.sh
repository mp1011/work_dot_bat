#!/usr/bin/env bash
# Cross-compiles work_dot_bat for Windows, macOS and Linux on amd64 and arm64.
#   scripts/build-release.sh [version]     (default: `git describe`, else "dev")
# Output: dist/work_dot_bat_<os>_<arch>[.exe]. Pure Go (CGO off), so nothing but the Go toolchain
# is needed; the binary carries the vendored scripts and templates (see assets.go).
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
version="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
out=dist
mkdir -p "$out"

targets="windows/amd64 windows/arm64 darwin/amd64 darwin/arm64 linux/amd64 linux/arm64"
for target in $targets; do
  os="${target%/*}"
  arch="${target#*/}"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  file="$out/work_dot_bat_${os}_${arch}${ext}"
  echo "building $file ($version)"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.Version=$version" -o "$file" ./src/cmd/work_dot_bat
done
echo "built $(echo $targets | wc -w) binaries in $out/"
