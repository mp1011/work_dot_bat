---
status: Done
subtask_of: plans/october_2026/automated_testing_improvements/plan.md
---

# Integrate Test Layers into work.bat Workflows

## Goal
The four test tiers are mapped directly to their intended workflow lifecycle triggers in `work_dot_bat`: `ultra_light` runs frequently for fast lint/type checks, `light` unit tests run after code changes in loop/daemon modes, `heavy` tests run upon task completion, and `ultra_heavy` quality gates run before pushing to origin.

## Context
Part of `agent_docs/plans/october_2026/automated_testing_improvements/plan.md`.
Currently, `automatedTests` in `cmd/work_dot_bat/checks.go` calls `RunAutomatedTests`, which runs `lightTest` and `heavyTest` from `work_dot_bat.config.json` via a single monolithic check list.
With the four defined tiers:
- UltraLight: linters, type checkers. Cheap tests to run often.
- Light: unit tests to run after a code change.
- Heavy: e2e / integration tests to run after a whole feature is ready.
- UltraHeavy: expensive quality gates to run before pushing code to origin.
Relevant files: `internal/work/app.go`, `internal/work/push.go`, `internal/batch/loop.go`, `internal/daemon/run.go`, `cmd/work_dot_bat/checks.go`.

## Requirements
1. Update `automatedTests` / `RunAutomatedTests` to read test layer commands from the project registry (`projects.json`), with fallback to legacy `work_dot_bat.config.json` test commands when `tests` is not defined in `projects.json`.
2. Map `prepare_for_push` to enforce the `ultra_heavy` suite (along with other mechanical gates) before allowing an origin push.
3. Update loop mode (`internal/batch/loop.go`) and daemon mode (`internal/daemon/run.go`) to run `light` tests after each iteration, and `heavy` tests before marking a task or plan ready for review.
4. Update the interactive launcher menu (`work`) to provide distinct options for running light tests, heavy tests, and the full pre-push test gate.

## Implementation Plan
Refactor `automatedTests` and `RunAutomatedTests` to accept an optional target layer or tier list. Wire `internal/work/push.go` to require the `ultra_heavy` tier. Update `batch.Loop` and `daemon.Runner` test hooks to trigger `light` vs `heavy` suites at the appropriate phase transitions. Update the interactive menu in `internal/work/app.go`.

## Testing Plan
- Pre-push gate: Running the push workflow executes `ultra_heavy` tests; if an `ultra_heavy` test command fails, push is blocked and reports the failure.
- Iteration test execution: In loop/daemon test runs, `light` is executed on code edits; a failing `light` test prevents proceeding to feature completion.
- Fallback compatibility: If `projects.json` has no `tests` block, existing projects with `work_dot_bat.config.json` (`lightTest`, `heavyTest`) continue running their configured commands as before.

## Verification Plan
`go test ./internal/work/... ./internal/batch/... ./internal/daemon/...` passes; `work_dot_bat prepare_for_push project=work_dot_bat` runs `ultra_heavy` tests before pushing.
