---
status: Done
subtask_of: plans/october_2026/remove_claims/plan.md
---

# Delete plan-claim

## Goal
`work_dot_bat plan-claim` and `plan-claim.sh` no longer exist, and nothing creates `.work_dot_bat/claims`.

## Context
Part of `plan.md`; do after subtasks 01 and 02. Delete `src/internal/planclaims/`; `plan-claim` command in `src/cmd/work_dot_bat/main.go:178`; `src/vendor/scripts/work_dot_bat/plan-claim.sh`, its `src/vendor/manifest.json` entry, the managed copy `.tools/plan-claim.sh`, and `plan-claim` mentions in `src/vendor/scripts/work_dot_bat/plan-status.sh` and `typescript/check-source-assertions.mjs`. Claims directory handling in `src/internal/repo`, `bootstrap`, `statemigrate`. Docs: `README.md`, `agent_docs/00_start_here.md`, `02_project_structure.md`, `deep_dives/task_menu.md`, both `prepare-for-push.md` copies (agent_docs and `templates/files/`). Check how `sync` removes managed files that leave the manifest so `.tools/plan-claim.sh` disappears on sync.

## Requirements
1. `work_dot_bat plan-claim ...` reports an unknown command; no `plan-claim.sh` is vendored or synced.
2. No code or docs mention claims outside `agent_docs/plans/`.
3. Fresh bootstrap and state migration do not create or require `.work_dot_bat/claims`.
4. `./.tools/run-checks.sh` passes (including managed-file/manifest checks).

## Implementation Plan
Delete the package and command, drop the vendored script and manifest entry, strip claims-dir handling, update docs and templates, run sync to confirm the managed copy is removed.

## Testing Plan
- CLI test: running `plan-claim check x` exits non-zero with "Unknown command".
- Bootstrap test: after bootstrap in an empty repo, `.work_dot_bat/claims` does not exist and no `plan-claim.sh` is written.
- Sync test: a repo with an old managed `plan-claim.sh` no longer has it after sync.

## Verification Plan
- `./.tools/run-checks.sh` green.
- `git grep -i "plan-claim\|planclaims"` returns nothing outside `agent_docs/plans/`.
