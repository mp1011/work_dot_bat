---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Go merge-back, cleanup, and commit workflows

## Goal
From the work menu, "Merge back into main" for a plan's branch works as today: gated with the same disabled-reason messages, shows the commits to be merged, stages a `--no-ff --no-commit` merge, reports conflicts without resolving them, and offers safe worktree and branch cleanup.

## Context
Part of `04_interactive_work.md`; depends on 03 and 04c. Source: the merge-back and cleanup parts of the interactive launcher (`resolveMergeBack`, `doMergeBackWorkflow`, `promptCleanup`), and `worktree-provision.mjs` / `worktree-common.mjs` (`mergeAvailability`, `mergeMenuLabel`, `isTreeDirty`, `currentBranch`, `commitsToMerge`, `conflictedPaths`, `isBranchMerged`, `stagedFiles`, `mergeBranchIntoMain`, `removeWorktreeAndBranch`, `commitWorktree`, `getIncompleteTasksForBranch`, `formatWorktreeSummary`). Never commits on the user's behalf; deletes use `git worktree remove` and `git branch -d` only, never force.

## Requirements
1. Gate and label text identical, in the same priority order (dirty tree first).
2. The incomplete-plans guard on the branch matches Node.
3. Merge outcomes `staged` / `conflict` / `failed` are reported; a conflict is left in progress.
4. Cleanup refusals from git are reported, never overridden.

## Implementation Plan
Extend `internal/worktree` with the pure gate and git helpers, and put the interactive flow in `internal/work`.

## Testing Plan
- In temp git repos: a clean mergeable branch ends with the changes staged and nothing committed; a conflicting branch lists the conflicted paths and leaves the merge in progress.
- A dirty main checkout disables the option with `commit or stash your changes first`.
- After a committed merge, cleanup removes the worktree and branch; with the worktree locked it reports the refusal and leaves the branch.

## Verification Plan
`go test ./...`; run the flow by hand in a scratch repo with a worktree branch.
