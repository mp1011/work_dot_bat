---
status: Done
subtask_of: plans/initial_port/04_interactive_work.md
verification: manual
---

# Go interactive agent launch and claim lifetime

## Goal
`work_dot_bat work project=<name>` lets the user pick a task, choose an action, and launches the agent interactively in the terminal on that plan; the plan/branch claim is held for the run and always released afterward, including when the agent fails or the user presses Ctrl-C.

## Context
Part of `04_interactive_work.md`; depends on 04a and 04b. Source: the interactive launcher in `src/` (`acquireLeafPlanClaim`, `releaseLeafPlanClaim`, `launchInteractiveAgent`, `doCommitWorkflow`, `doSetAutoWorkflow`, `main`, the Step 2 action menu), `work-common.mjs` (`getTaskMenuOptions`, `runCommand`, `commitIfDirty`, `updateTaskStatus`, plan-breakdown detection: `getPlanBreakdownInfo`, `detectPlanBreakdown`, `treeHasNonStatusChanges`, `isWorkingTreeDirty`), `batch-common.mjs` (`withClaimRelease`). Agent processes use `os/exec` with stdin/stdout/stderr passed straight through on Windows, macOS and Linux. Registers the `work` tool in `internal/tools`. The push workflow is subtask 08, merge-back is 04d.

## Requirements
1. Step 2 action menu options and their availability match `getTaskMenuOptions`.
2. Claim acquired (as `launcher`, with the launcher's own pid) before launch and released via deferred cleanup on normal exit, agent failure and SIGINT.
3. After the agent exits, status and breakdown detection and the optional commit and set-auto flows behave as today.
4. The agent receives the TTY unmodified (no input swallowed by the launcher).

## Implementation Plan
New package `internal/work` over `planclaims`, `planstatus`, `agentcmd`, `taskmenu`. Signal handling lives in one place that runs the release.

## Testing Plan
- With a stub agent executable that exits 1, the claim files under `.work_dot_bat/claims/` are gone afterward; same after a simulated interrupt (the test cancels the context). Observed in the claims directory.
- After a stub agent that edits a plan file, breakdown detection reports the same classification as the Node cases ported from `work-common.test.ts`.
- Menu options for a task with and without a worktree, and for a parent task, match golden lists.

## Manual Verification
1. Create a scratch project with a stub interactive agent script that reads one line from stdin and echoes it.
2. Run `work_dot_bat work project=<scratch>`, pick a task and launch the agent.
3. Type a line into the agent and confirm it arrives intact; the launcher must not swallow or duplicate input.
4. Run it again, press Ctrl-C while the agent runs, and confirm `plan-claim list` shows no claim afterward.

## Verification Plan
`go test ./...`; run the manual steps above and watch plan status and claim transitions.
