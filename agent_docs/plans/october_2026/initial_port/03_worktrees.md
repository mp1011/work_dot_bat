---
status: Done
subtask_of: plans/initial_port/plan.md
---

# Worktree creation and info in Go

## Goal
`work_dot_bat create_worktree` provisions a git worktree for a plan (branch, configured files, dependency install per project config) and `worktree-info` reports worktree/branch state, the same as today.

## Context
Depends on subtask 01.
Source: `src/create_worktree.js`, `src/lib/worktree-common.mjs`, `worktree-provision.mjs`, `auto-git.mjs`, `vendor/.../worktree-common.mjs`, `worktree-info.mjs`, `worktree-info.sh`, and their tests including `worktree-config.test.ts`. Shell out to the `git` binary (no libgit2). Handle Windows paths and symlink fallbacks.

## Requirements
1. Create or reuse a worktree and branch for a plan, honoring per-project config.
2. Provisioning steps (copy/symlink files, install commands) run in the new worktree.
3. `worktree-info` output matches the current format.
4. Works with Windows and POSIX path semantics.

## Implementation Plan
Wrap git behind a small runner interface so tests can use real temp repos.

## Testing Plan
- In a temp git repo, creating a worktree for plan X yields a directory on branch X containing the configured provisioned files; observed via `git worktree list` and the filesystem.
- Re-running is idempotent (no error, same result).
- `worktree-info` run from the main checkout vs. a linked worktree reports the right role and branch.

## Verification Plan
`go test ./...`; manually create and remove a worktree for this repo's `initial_port` plan in a scratch location.

## Implementation Notes
- `internal/worktree`: naming (`PathFor`), the pure `Decide` classifier, `Ensure` (create or reuse; never deletes), `Bootstrap` (install command, plan_status.js shim, copy-if-absent of `worktreeCopyFiles` from the main checkout), `StartDevServer` (detached; per-OS process attrs), `GetInfo`/`FormatInfo`, and the `create_worktree` app. Git is shelled out behind an injectable `Runner`; tests run real git in temp repos.
- Entry points: `work_dot_bat create_worktree [branch=<name>|plan=<path>]` (prompts when neither is given) and the repo-level `work_dot_bat worktree-info` (no project registry, like `plan-status`).
- Verified: `worktree-info` output is byte-identical to the Node script from both a main checkout and a linked worktree in a scratch repo. Plan mode on a subtask lands it on its parent's branch, the bootstrap copy runs, and a rerun reuses the worktree without re-running anything.
- Differences from Node: (1) the plan picker here is a plain numbered list of the open backlog; the shared menu (priority grouping, write-in, paste-path, parked suggestions) arrives with subtask 04 and should replace `App.PickPlan`. (2) The shim regeneration is done in-process via `planstatus.WriteShim` instead of spawning the vendored script. (3) On git failure `worktree-info` prints git's message and exits 1 rather than a Node stack trace. (4) Commands are exec'd directly (Go resolves `npm.cmd` on Windows); only an explicit `shell: true` in the config uses a shell.
- Not ported here (belongs to 04/05, which consume them): the commit, merge-back and removal helpers (`commitWorktree`, `mergeBranchIntoMain`, `removeWorktreeAndBranch`, `isBranchMerged`, merge gating text) and `setStatusInWorktree`.
- This repo has no commits yet, so the manual "create a worktree for `initial_port`" check was done in a scratch repo instead.
