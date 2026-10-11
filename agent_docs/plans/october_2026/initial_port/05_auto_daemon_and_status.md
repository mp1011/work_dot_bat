---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Auto daemon and status in Go

## Goal
`work_dot_bat auto_daemon project=<name>` unattended picks eligible `auto: true` plans, runs an agent per plan in its own worktree, records outcomes, and `auto_status` reports current state, as today.

## Context
Depends on subtasks 02–04.
Source: `src/auto_daemon.js`, `src/auto_status.js`, `src/lib/daemon-run.mjs` (899 lines), `daemon-plans.mjs`, `daemon-agent.mjs`, `auto-flag-writer.mjs`, and tests. Use goroutines and `context` for concurrency and cancellation. Keep the `AUTO_OUTCOME:` parsing contract.

## Requirements
1. Plan eligibility selection and ordering match current rules.
2. One worktree and agent run per plan; `AUTO_OUTCOME: done|subtasks|blocked` is parsed and applied to plan status.
3. Graceful shutdown on signal; no orphaned agent processes (including Windows process-tree kill).
4. `auto_status` output matches.

## Implementation Plan
A pure planner (which plans, in what order) separate from the runner; the runner takes an agent-runner interface.

## Testing Plan
- Planner: a temp plans tree with mixed statuses/flags yields the expected queue (ported tests).
- Runner with a fake agent that prints each `AUTO_OUTCOME` variant: resulting plan status and flag file are as expected.
- Cancellation: cancelling the context terminates a long-running fake agent and its child; observed by process absence.
- `auto_status` golden output.

## Verification Plan
`go test ./...`; dry-run the daemon on a scratch project with a stub agent.
