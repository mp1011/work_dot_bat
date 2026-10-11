---
status: Done
---

# Immediate plan: automation failures feed straight back to the agent

## Goal
`agent_docs/plans/immediate.md` is a special, fixed plan file that acts as a one-slot "fix this now" inbox. Whenever an automation fails (tests, checks, the push gate, the daemon's verification), the failure output is written into this file. The next time `work` runs, if `immediate.md` exists and is not `Done`, it is picked for the agent automatically, ahead of the normal task menu. When the automation later passes, the file is marked `Done` and its content is erased, so it is ready to be reused.

## Context
- This file is the plan that *builds* the feature. The runtime file it describes is the separate, fixed `agent_docs/plans/immediate.md`, which does not exist until the first automation failure.
- Subtasks, in order: `01_record_failures.md`, `02_auto_pick.md`, `03_auto_clear.md`.
- Plan files carry status in YAML frontmatter and must only be changed through `.tools/plan-status.sh` (`add`, `set`); never hand-edit the frontmatter. `immediate.md` is a normal plan file as far as status goes.
- Task listing and runnable filtering live in `src/internal/taskmenu/list.go` (`RunnableAndGrouped`); the interactive launcher is `src/internal/work/app.go`; loop mode is `src/internal/batch` / `work.App.loopUntilDone`; the daemon is `src/internal/daemon/run.go`.
- The shared automation entry is `checks.Env.RunAutomatedTests` (`src/internal/checks/runchecks.go`), used by the daemon, loop mode, the launcher's test options and the push workflow (`src/internal/work/push.go`). It returns only failing check names today, so the failure *output* has to be captured as well.
- `immediate.md` is special: it is a fixed path, it is reused rather than created per failure, and "Done" is not terminal for it: the Work script erases the body when it sets `Done`, and a new failure sets the status back to `Not Started` and writes a fresh body. Only `Done` (not `Not Started`, `In Progress`, `Ready for Review`, `Blocked`) means "nothing pending".
- Claims (`plan-claim.sh`) still apply: do not pick it if another session holds a live claim on it.

## Requirements
1. **Fixed path.** The file is always `agent_docs/plans/immediate.md` (in the project root, or in the active worktree when one is in use). It is created on the first failure if absent, via `plan-status.sh add`, and reused afterwards.
2. **Failure capture.** When an automation fails, the failing automation's name, the command, and its error output (truncated to a sane limit, tail preserved) are written into `immediate.md` using the plan template shape (Goal: make this automation pass; Context: the captured output). If the file already holds an unresolved failure, the new failure is appended as a new section rather than overwriting it. The status is set to `Not Started` (or left `In Progress` if already being worked) via `plan-status.sh`.
3. **Automatic pick.** If `immediate.md` exists and its status is anything other than `Done`, the `work` launcher, loop mode and the daemon pick it first without showing the menu (interactive mode shows a one-line notice, "Immediate plan pending: picking agent_docs/plans/immediate.md"). If it is `Done` or absent, behavior is unchanged.
4. **Auto-completion.** When the automation that caused the failure next passes, the Work script moves the file to `Done` through `plan-status.sh`, and its body (everything after the frontmatter) is erased. The frontmatter block stays intact. A pass of an *unrelated* automation must not clear a failure of a different one.
5. **`Done` is set by the Work script, never the agent.** The Work script (launcher, loop mode, daemon) runs `plan-status.sh set agent_docs/plans/immediate.md "Done"` itself when it sees the automation pass, then erases the body. The agent never sets `Done` and never edits the file's status or body; it just fixes the failure. This is the single exception to the "only the user confirms Done" rule, and `00_start_here.md` documents it.
6. **Not a menu duplicate.** `immediate.md` does not appear as a regular task in the Step 1 menu while it is `Done`, and is not eligible for the daemon's normal planner path under its regular rules (it is handled by requirement 3).
7. **Safe on failure to write.** If writing `immediate.md` itself fails, the original automation failure is still reported to the user and the exit code is unchanged.

## Implementation Plan
- Add a small package (e.g. `internal/immediate`) that owns the fixed path, `Record(failure)`, `Pending() (path, bool)` and `Clear(automation)`. Keep file I/O and the status calls behind injectable fields so tests can use temp directories and a fake status runner, matching how the daemon is structured.
- Extend `RunAutomatedTests` (or a thin wrapper) to return structured results: automation name, command, exit status, captured output. Keep the existing failing-names return for current callers. Call `Record` on failure and `Clear` on a pass from the same call sites (daemon, loop, launcher tests, push gate).
- Teach the launcher, loop mode and daemon to consult `Pending()` before selecting a task. Decide in the first step how it interacts with `RunnableAndGrouped` (preferred: a pre-check, not a special case inside the list).
- Record which automation owns the failure (a marker line in the body, e.g. `AUTOMATION: <name>`), so `Clear` can match it and sections for several automations can coexist; the file goes to `Done` only when all recorded automations pass.
- Add a short section to `agent_docs/00_start_here.md` and `agent_docs/deep_dives/task_menu.md` describing the file and the Done exception.

## Testing Plan
- Failing automation with no existing file: run the recording path with a fake failing check producing known output; the test reads `immediate.md` and asserts it exists, has `status: Not Started`, and contains the check name and that output.
- Second failure from a different automation while the first is unresolved: the file contains both sections and still has a single frontmatter block.
- `Pending()` returns true for each non-`Done` status and false for `Done` and for a missing file; tested against real temp files.
- Launcher/loop/daemon selection: with a pending `immediate.md` and other runnable plans, the chosen plan is `immediate.md` (assert on the selected path from the injected picker/agent fake); with it `Done`, the normal selection is unchanged.
- Passing automation: after a pass of the recorded automation, the file's status is `Done` and the body is empty while the frontmatter is intact; a pass of an unrelated automation leaves both the status and the body untouched.
- Multiple recorded automations: the file only goes `Done` after the last of them passes.
- A claimed `immediate.md` (live claim by another session) is not picked.
- Write failure (read-only directory): the automation failure is still returned and the exit code is unchanged.

## Verification Plan
- `go build ./... && go vet ./... && go test ./...` from `src/`.
- `.tools/run-checks.sh` passes.
- Manual: break a test in a scratch project, run the push gate, confirm `agent_docs/plans/immediate.md` appears with the error; run `work` and confirm it is picked without the menu; fix the test, re-run the gate, confirm the file is `Done` and empty.
