#!/usr/bin/env bash
# Smoke test for a built binary, in a scratch git repo:
#   scripts/smoke.sh <path-to-work_dot_bat-binary> [expected-version]
# Exit 0 when --version, list-tasks, sync --dry-run, bootstrap and run-checks all behave.
set -euo pipefail

bin="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
expected="${2:-}"

version="$("$bin" --version)"
echo "$version"
case "$version" in "work_dot_bat "*) ;; *) echo "unexpected --version output" >&2; exit 1 ;; esac
if [ -n "$expected" ] && [ "$version" != "work_dot_bat $expected" ]; then
  echo "expected version $expected" >&2
  exit 1
fi

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
cd "$scratch"
git init -q
git config user.name smoke
git config user.email smoke@example.com

# Tools run against a registered project: a throwaway registry names the scratch repo.
path="$scratch"
command -v cygpath >/dev/null 2>&1 && path="$(cygpath -m "$scratch")"
printf '{ "projects": [ { "name": "smoke", "path": "%s" } ] }
' "$path" > "$scratch/../registry-$$.json"
export WORK_DOT_BAT_REGISTRY="$scratch/../registry-$$.json"
trap 'rm -rf "$scratch" "$WORK_DOT_BAT_REGISTRY"' EXIT

"$bin" bootstrap project=smoke >/dev/null
test -f work_dot_bat.config.json
test -f agent_docs/00_start_here.md
test -f scripts/work_dot_bat/plan-status.sh

# A freshly bootstrapped repo has nothing to update.
"$bin" sync project=smoke --dry-run | grep -q "Everything is up to date."

mkdir -p agent_docs/plans
printf '# GOAL\nsmoke\n' > agent_docs/plans/smoke.md
"$bin" plan-status add plans/smoke.md >/dev/null
"$bin" list-tasks | grep -q "plans/smoke.md"

# Only the checks that need no Node project: skills and plan drift.
printf '{ "checks": ["skills"] }\n' > work_dot_bat.config.json
"$bin" run-checks --skip-tests | grep -q "All automated tests passed"
echo "smoke ok"
