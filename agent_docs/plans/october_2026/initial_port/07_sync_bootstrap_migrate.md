---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# sync, bootstrap, migrate_state, rewrite_paths in Go

## Goal
`work_dot_bat sync`, `bootstrap`, `migrate_state` and `rewrite_paths` prepare and maintain a target repo (managed files, agent docs, skills, safety hook, baselines, pre-push hook) with the same dry-run / `--yes` / `--apply` behavior. Templates and vendored scripts are embedded in the binary.

## Context
Depends on subtasks 01–02.
Source: `src/sync.js`, `bootstrap.js`, `migrate_state.js`, `rewrite_paths.js`, `src/lib/sync.mjs`, `bootstrap.mjs`, `managed-file.mjs`, `vendor-manifest.mjs`, `vendored-scripts.mjs`, `state-migration.mjs`, `path-rewrite.mjs`, `safety-hook.mjs`, `scripts/vendor-bump.mjs`, `templates/`, `vendor/`. Use `embed.FS`. Resolve the shim decision from subtask 02 (what gets copied into target repos). The safety hook currently embeds a `node -e` command in `.claude/settings.json`; replace it with a binary subcommand.

## Requirements
1. Dry-run prints the same plan of changes; `--yes` applies without committing.
2. SHA-based manifest drift detection, and a Go replacement for `vendor:bump` / `vendor:check`.
3. Legacy `.tools/` and `.claims/` migration; rewrite of `scripts/<x>.sh` references.
4. The safety hook blocks the same commands (commit/push, prepare_for_push, work) with the same messages.

## Implementation Plan
Port managed-file logic first (pure), then the commands. Embed assets at build time.

## Testing Plan
- `sync --dry-run` on a temp repo lists files to add/update and writes nothing; `--yes` creates them with manifest SHAs matching.
- A locally modified managed file is detected as drifted and not clobbered without confirmation.
- `migrate_state --apply` moves legacy dirs into `.work_dot_bat/`; observed on the filesystem.
- Safety hook: hook JSON for `git push` yields exit 2 and the message; for `ls`, exit 0.

## Verification Plan
`go test ./...`; `work_dot_bat bootstrap --dry-run` against a scratch repo.
