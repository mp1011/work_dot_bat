#!/usr/bin/env bash
# Automatically adds work_dot_bat/.tools to PATH in shell profile (~/.bashrc, ~/.zshrc, etc.)
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_dir="$repo_root/.tools"

if [ ! -d "$bin_dir" ]; then
  echo "Error: .tools directory not found at $bin_dir" >&2
  exit 1
fi

echo "work_dot_bat repository root: $repo_root"
echo "Target directory to add to PATH: $bin_dir"

# Detect appropriate profile file
profile=""
if [ -n "${ZSH_VERSION:-}" ] || [ "$(basename "${SHELL:-}")" = "zsh" ]; then
  profile="$HOME/.zshrc"
elif [ -f "$HOME/.bashrc" ]; then
  profile="$HOME/.bashrc"
elif [ -f "$HOME/.bash_profile" ]; then
  profile="$HOME/.bash_profile"
elif [ -f "$HOME/.profile" ]; then
  profile="$HOME/.profile"
else
  profile="$HOME/.bashrc"
fi

export_line="export PATH=\"$bin_dir:\$PATH\""

if grep -qsF "$bin_dir" "$profile" 2>/dev/null; then
  echo "[OK] $bin_dir is already in $profile"
else
  echo "" >> "$profile"
  echo "# work_dot_bat" >> "$profile"
  echo "$export_line" >> "$profile"
  echo "[SUCCESS] Added $bin_dir to $profile"
fi

export PATH="$bin_dir:$PATH"
echo "[OK] Updated current shell PATH."

if command -v work_dot_bat >/dev/null 2>&1; then
  echo "work_dot_bat resolved to: $(command -v work_dot_bat)"
  work_dot_bat --version || true
fi
