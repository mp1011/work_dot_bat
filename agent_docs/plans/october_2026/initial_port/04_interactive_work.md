---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Interactive work launcher in Go

## Goal
`work_dot_bat work` presents the same interactive task/plan picker and launches the configured agent on the chosen plan, holding and releasing the plan/branch claim, as the current `work` launcher does.

## Context
Depends on subtasks 01–03.
Source: `src/work.js` (711 lines), `src/lib/work-common.mjs` (1354 lines), `loop-work.mjs`, `batch-common.mjs`, `agent-settings.mjs`, `check-list.mjs`, and tests. This is the largest port: split by responsibility (menu, agent spawn, claim lifetime, loop mode) per the monolith rules. Agent spawning uses `os/exec` with TTY pass-through on all three OSes.

## Requirements
1. The menu of plans/tasks and the mode selection match in options and ordering.
2. The agent launch command is built from engine config; the prompt text and the launcher-claim note match current text.
3. The claim is acquired before launch and released in a deferred cleanup, including on Ctrl-C.
4. Loop/batch modes behave as today.

## Implementation Plan
Extract pure functions for menu building and command construction first; keep the TTY shell thin.

## Testing Plan
- Command builder: given engine config and a plan, returns the exact argv/prompt (golden tests ported from `work-common.test.ts`).
- Menu: given a temp plans tree, the rendered list and selection mapping match golden output.
- Claim lifetime: with a fake agent executable that exits 1, the claim file is gone afterward; same after an interrupt. Observed in the claims directory.
- TTY pass-through: manual verification (`verification: manual`) with written steps.

## Verification Plan
`go test ./...`; manually run against a scratch project with a stub agent script and confirm the plan status and claim transitions.
