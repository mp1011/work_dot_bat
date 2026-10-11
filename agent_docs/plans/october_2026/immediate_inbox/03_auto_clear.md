---
status: Done
subtask_of: plans/october_2026/immediate_inbox/plan.md
---

# Passing automation completes and clears immediate.md

## Goal
When the automation that failed passes on a later run, the user sees `agent_docs/plans/immediate.md` become `Done` with an empty body, and `work` goes back to showing the normal menu. The agent never sets `Done`; the Work script does.

## Context
Part of `agent_docs/plans/october_2026/immediate_inbox/plan.md`; depends on `01_record_failures.md` (the `AUTOMATION:` markers) and `02_auto_pick.md`.
- Same call sites as subtask 01 (daemon, loop mode, launcher tests, push gate).
- Status through `.tools/plan-status.sh set agent_docs/plans/immediate.md "Done"` only. This is the one exception to "only the user confirms Done"; document it.
- The agent working the file is told not to touch its status or body; the Work script owns both.

## Requirements
1. When an automation recorded in `immediate.md` passes, its section is removed.
2. When the last recorded automation has passed, the Work script sets `Done` through `plan-status.sh` and erases everything after the frontmatter. The frontmatter stays intact.
3. A pass of an automation that is not recorded changes nothing.
4. A later failure after `Done` sets `Not Started` and writes a fresh body (subtask 01 behavior), so the file is reusable.
5. `agent_docs/00_start_here.md` and `agent_docs/deep_dives/task_menu.md` describe the file and the Done exception, and the prompts given to the agent for this plan say not to set its status.

## Implementation Plan
Add `Clear(automation)` to `internal/immediate` and call it on a pass from every call site that calls `Record` on a failure. Update the docs and the agent prompt text.

## Testing Plan
- Recorded failure of automation A, then A passes: the file's status is `Done`, the body is empty and the frontmatter is intact (read from a temp dir with a real status runner).
- A and B recorded; A passes: the file is still not `Done` and only B's section remains; B passes: `Done` and empty.
- An unrelated automation passes: file bytes unchanged.
- After `Done`, a new failure: status `Not Started`, body contains only the new failure.
- After a pass, the launcher shows the normal menu (the picker is called).

## Verification Plan
`go build ./... && go vet ./... && go test ./...` from `src/`; `.tools/run-checks.sh`. Manually: break a test, run the push gate, run `work` (picks `immediate.md`), fix the test, re-run the gate, and confirm the file is `Done` and empty.
