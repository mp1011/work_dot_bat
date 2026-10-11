---
status: Ready for Review
---

# PROBLEM


We attempt to prevent two agents from working on the same task, but this has proven more trouble than its worth. It should be the human's responsibility to make sure they don't ask two agents to work on the same plan.

## Goal
Remove the concurrency check mechanism that prevents agents from working on a plan. The human launcher (`work`) should not block or skip task execution based on held claims or append claim-management notes to agent prompts, agents should not perform Step Zero concurrency checks or acquire/release claims during their runs, and the `concurrent-check` subcommand should be removed.

## Context
- The previous design attempted to prevent race conditions when two agents run simultaneously by acquiring claims on leaf tasks before launching agents interactively in `work`, adding notes to agent prompts about claims (`LauncherClaimNote`), and instructing agents to perform Step Zero concurrency checks (`scripts/work_dot_bat/plan-claim.sh concurrent-check <plan>`).
- This mechanism created friction: launcher claims blocked agents when `concurrent-check` reported `BLOCKED`, crashed sessions left claims that skipped tasks, and prompt notes added unnecessary noise and constraints.
- Managing concurrency is the human's responsibility.

## Requirements
1. **Interactive Work Launcher (`src/internal/work`)**:
   - In `interactiveWork` (`app.go`), launch agents directly for leaf tasks without checking `WithClaim` or skipping with `[SKIP] The plan/branch claim is held by someone else. Not starting this task.`.
   - Stop appending `agentcmd.LauncherClaimNote` to the agent prompt.
   - Loop mode (`RunLoopUntilDone`) in `app.go` should not pass claim acquire/release hooks that block unattended runs on held claims.
2. **Agent Prompts (`src/internal/agentcmd`)**:
   - Deprecate/remove `LauncherClaimNote`.
   - In `BuildDoWorkYoloPrompt`, remove instructions regarding claims, skipping acquire/release, and running `concurrent-check`. Keep only the instruction to not mark "Ready for Review" when post-run automated verification is configured.
   - In `BuildRetryPrompt`, remove the claim note suffix.
   - In `src/internal/batch/loop.go`, do not append `LauncherClaimNote` to failure follow-up prompts.
3. **Agent Instructions and Core Docs**:
   - `agent_docs/instructions/high_trust/do-work-yolo.md`: Remove STEP ZERO's concurrent-work check (running `concurrent-check`, `git status --short` target files check, and claim `acquire`/`release`).
   - `agent_docs/instructions/low_trust/do-work.md`: Remove Step zero check for concurrent work.
   - `agent_docs/instructions/high_trust/next-subtask-yolo.md`: Remove claim check step.
   - `agent_docs/00_start_here.md`: Remove Rule 4 ("Check for concurrent work before starting a plan...").
   - Update corresponding templates in `templates/files/agent_docs/`.
4. **Plan Claims CLI (`src/internal/planclaims`)**:
   - Remove `concurrent-check` subcommand from `src/internal/planclaims/cli.go` and `ClassifyConcurrentWork` from `src/internal/planclaims/claims.go`.
   - Update comments in `.tools/plan-claim.sh` and vendored scripts.
5. **Testing**:
   - Update tests in `src/internal/work/app_test.go`, `src/internal/agentcmd/agentcmd_test.go`, `src/internal/batch/batch_test.go`, and `src/internal/planclaims/planclaims_test.go` to verify the new behavior.

## Implementation Plan
1. Update `src/internal/planclaims` to remove `concurrent-check` and `ClassifyConcurrentWork`.
2. Update `src/internal/agentcmd` and `src/internal/batch` prompt generation and tests.
3. Update `src/internal/work` interactive launcher to remove claim gating and prompt claim notes.
4. Update agent documentation (`00_start_here.md`, `do-work-yolo.md`, `do-work.md`, `next-subtask-yolo.md`) and templates.
5. Update all associated unit tests and verify with `./.tools/run-checks.sh`.

## Testing Plan
- `go test ./src/internal/planclaims/...`: Verify `concurrent-check` is removed while remaining claim commands work.
- `go test ./src/internal/agentcmd/...`: Verify prompt generation does not include claim / concurrency check notes.
- `go test ./src/internal/batch/...`: Verify batch and loop prompt generation.
- `go test ./src/internal/work/...`: Verify `interactiveWork` launches even if an external claim is present, and prompt has no claim note.
- `./.tools/run-checks.sh`: Full test suite passes.

## Verification Plan
- `./.tools/run-checks.sh` green.
- Verify `git grep concurrent-check` returns no remaining references in active code or instructions.