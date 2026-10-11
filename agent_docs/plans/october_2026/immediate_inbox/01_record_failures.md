---
status: Done
subtask_of: plans/october_2026/immediate_inbox/plan.md
---

# Record automation failures in immediate.md

## Goal
When an automation fails (the push gate, loop-mode tests, the daemon's verification, the launcher's test options), the user finds `agent_docs/plans/immediate.md` containing which automation failed, the command, and the tail of its error output. Nothing else about `work` changes yet.

## Context
Part of `agent_docs/plans/october_2026/immediate_inbox/plan.md`.
- `checks.Env.RunAutomatedTests` (`src/internal/checks/runchecks.go`) returns only failing check names; the failure output must be captured too. Keep the existing return for current callers.
- Call sites: `src/cmd/work_dot_bat/checks.go`, the daemon (`src/internal/daemon/run.go`), loop mode (`work.App.loopUntilDone`), `src/internal/work/push.go`.
- Status changes only through `.tools/plan-status.sh` (`add`, `set`); never hand-edit frontmatter.
- Introduce `internal/immediate` with the fixed path, `Record(failure)` and `Pending() (path, bool)`; file I/O and the status runner behind injectable fields, as the daemon does. Do not erase or complete anything here (subtask 03).

## Requirements
1. The path is always `agent_docs/plans/immediate.md` in the project root. Runs in a worktree (the daemon's) neither record nor clear: the file would pollute the branch diff and the daemon retries its own failures. It is created on the first failure through `plan-status.sh add`.
2. The body follows the plan template (Goal: make the named automation pass; Context: automation name, command, captured output with the tail preserved and a size cap). A marker line `AUTOMATION: <name>` identifies the owner.
3. A second failure from a different automation while the file is not `Done` appends a new section; there is still exactly one frontmatter block. A repeat failure of the same automation replaces its own section.
4. After recording, the status is `Not Started`, except it stays `In Progress` if the file is already being worked.
5. If writing the file fails, the original failure is still reported and the exit code is unchanged.

## Implementation Plan
Add `internal/immediate`, extend `RunAutomatedTests` (or wrap it) with structured results, and call `Record` from each call site on failure.

## Testing Plan
- A fake failing check with known output: after the run, `immediate.md` exists, has `status: Not Started`, and contains the check name, command and that output (read from a temp project dir).
- Two failures from different automations leave one frontmatter block and both `AUTOMATION:` sections; a repeat of the first replaces only its own section.
- Output over the size cap keeps the tail and drops the head.
- A read-only target directory: the failure list returned and the exit code are the same as without the feature.

## Verification Plan
`go build ./... && go vet ./... && go test ./...` from `src/`. Manually break a test in a scratch project, run the push gate, and confirm `agent_docs/plans/immediate.md` appears with the error.
