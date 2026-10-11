---
status: Done
subtask_of: plans/october_2026/immediate_inbox/plan.md
---

# Work picks a pending immediate.md automatically

## Goal
When `agent_docs/plans/immediate.md` exists and is not `Done`, running `work` goes straight to it for the agent, with a one-line notice, instead of showing the task menu. The loop mode and the daemon do the same.

## Context
Part of `agent_docs/plans/october_2026/immediate_inbox/plan.md`; depends on `01_record_failures.md` (`immediate.Pending()`).
- Menu and runnable filtering: `src/internal/taskmenu/list.go` (`RunnableAndGrouped`), `menu.go`. Launcher: `src/internal/work/app.go`. Loop: `work.App.loopUntilDone`. Daemon planner: `src/internal/daemon/plans.go`, `run.go`.
- Claims (`plan-claim.sh`) still apply. Prefer a pre-check before selection rather than a special case inside the list.

## Requirements
1. With a pending `immediate.md`, `work` skips the Step 1 menu and prints `Immediate plan pending: picking agent_docs/plans/immediate.md`, then continues to the normal action step for that plan.
2. Loop mode inherits the pick because it starts after the launcher's selection. The daemon is deliberately unchanged: it works in branch worktrees, which never record or clear (see `01_record_failures.md`), so it has no pending file to pick.
3. If it is `Done` or absent, behavior is exactly as before, and a `Done` `immediate.md` does not show up in the menu.
4. A live claim by another session on it means it is not picked; the normal selection proceeds and the notice says why.
5. Its status is any value other than `Done` counts as pending (`Not Started`, `In Progress`, `Ready for Review`, `Blocked - User Input Required`).

## Implementation Plan
Consult `Pending()` at the start of selection in the launcher, loop and daemon. Hide `immediate.md` from the regular list and the daemon's eligibility rules.

## Testing Plan
- Launcher with a pending file and other runnable plans: the injected picker is never called and the launched plan path is `immediate.md`; the notice is printed.
- Same with each non-`Done` status; with `Done` or no file, the picker is called and the menu output is unchanged.
- Daemon planner with both a pending `immediate.md` and eligible plans: `immediate.md` is the first plan run.
- A live claim held by another owner: the picker is called and the notice names the claim.

## Verification Plan
`go build ./... && go vet ./... && go test ./...` from `src/`. Manually: with a failure recorded by subtask 01, run `work` and confirm it starts on `immediate.md` without the menu.
