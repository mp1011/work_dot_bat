# Testability Escalation

Shared doctrine for both trust levels' `small-enhancement-tdd.md`, `red-green-refactor-iteration.md`, `task-breakdown.md`, `do-work-yolo.md`, and `next-subtask-yolo.md`. `scripts/work_dot_bat/check-source-assertions.sh` gates new source-text assertions at pre-push; this doc is the sanctioned way to say *"I could not observe this"* or *"I could not reproduce this"* instead of reaching for the next-cheapest green artifact.

## Observability (writing tests)

**Before writing a test, name the observable** — the inputs, the outputs, and how the test observes them. If you cannot name one, you have a placeholder, not a test.

**Never assert on the source text of a file.** `expect(content).toContain('x')` passes when the behavior is broken and fails when a variable is renamed.

**If the behavior is not observable in the harness, make it observable** — extract the logic into a pure function that takes its inputs as arguments and returns its outputs. Most "untestable" UI/animation/canvas code is a pure function wrapped in an unobservable shell. This reshaping is in scope for the task, not a separate task.

**If extraction is not viable, stop and ask the user** — a supported outcome, not a failure. Use `AskUserQuestion` with concrete options: (a) refactor further to make it testable, (b) cover it with a heavy (end-to-end) test instead, (c) manual verification only — set `verification: manual` and write the `## Manual Verification` steps, (d) the user has another approach.

**In non-interactive modes** (`do-work-yolo`, `next-subtask-yolo`, the Auto Daemon) you cannot ask. Instead: mark the plan `Blocked - User Input Required` via `scripts/work_dot_bat/plan-status.sh set <path> "Blocked - User Input Required"` — *never* `Ready for Review` — add a `## Blocked — Needs User Input` section stating exactly what you could not observe and which options you see, and register a suggestion plan under `agent_docs/plans/agent_suggestions/`. An unattended run that stops and says "I could not verify this" is a success. One that writes a string-matching test and reports done is a failure.

## Reproduction (bug fixes)

**Before fixing a bug, reproduce the failure** — the same discipline as naming an observable before writing a test. A fix written against a failure you have only ever seen in someone else's log is a guess.

**A green run of a test that already passed here is not evidence.** If it passed before your change, running it again after proves nothing. Report that honestly rather than as verification.

**An unchanged failure means an inert fix.** Identical values after a change — same pixels, same numbers, same message — is proof the change did nothing. Discard it and re-diagnose; never layer a second guess on top.

**If it only fails elsewhere** (CI, another OS, another browser build), bring the trigger here instead of guessing at it: force the differing condition locally — wider font metrics, a narrower viewport, a slower clock — until it fails, then fix, then confirm the repro flips. Land that forced condition as a companion test that asserts up front that the trigger really holds, so it cannot pass vacuously.

**If you cannot reproduce it, do not fix it.** Interactive: ask the user with concrete options. Non-interactive: mark the plan `Blocked - User Input Required`, never `Ready for Review`, with a `## Blocked — Could Not Reproduce` section naming what you tried and what you would need (a fresh log with commit SHA, viewport, browser build).
