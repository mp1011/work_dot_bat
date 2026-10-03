The user provides a folder of subtasks, or a parent plan file whose subtasks carry `subtask_of: <that parent>` (what `work.bat`'s "Work on Next Subtask" passes). If neither, ask them for one.

Run `scripts/work_dot_bat/plan-status.sh list-tasks` to see the subtask set and each one's status.

1. Pick the next not-completed subtask in ascending `priority:` order (P1 highest; unprioritised subtasks last, in filename order) — matching how `work.bat` lists them.
   - **Step zero — check for concurrent work:**
     1. `scripts/work_dot_bat/plan-claim.sh check <subtask_path>` (use `check`, **not** `concurrent-check` — a breakdown's subtasks share the parent's worktree branch, so siblings marked `@ <branch>` are the expected state you are continuing, not a collision). If it exits 1 (held), stop and report the holder's kind, label, pid, branch, worktree, and age exactly as printed.
     2. `git status --short`. If the subtask's own target files (named in its "Technical Requirements" / "Research" sections) already show modified in your checkout, stop and report.
     3. Both pass: acquire with `scripts/work_dot_bat/plan-claim.sh acquire <subtask_path> --owner next-subtask-yolo`. This run holds the claim through `do-work-yolo.md` step 2 (which must not double-acquire) and releases it at step 3. Stale claims self-recover (TTL `AGENT_TTL_MS` 4h, `plan-claim.sh list` staleness, `--steal`).
   - Set the subtask `"In Progress"`: `scripts/work_dot_bat/plan-status.sh set <subtask_path> "In Progress"`.
   - Set the parent plan `"In Progress"` too: `scripts/work_dot_bat/plan-status.sh set <parent_plan_path> "In Progress"`.
2. Invoke `agent_docs\instructions\high_trust\do-work-yolo.md` to complete the task — it skips its own step zero since you hold the claim.
3. Review the output with the user.
   - If `do-work-yolo.md` escalated per `agent_docs\instructions\testability_escalation.md` (subtask `"Blocked - User Input Required"` with a `## Blocked — Needs User Input` section), do not override — surface the blocker. Release the claim: `scripts/work_dot_bat/plan-claim.sh release <subtask_path>`.
   - Otherwise set the subtask `"Ready for Review"` via `scripts/work_dot_bat/plan-status.sh set <subtask_path> "Ready for Review"` and release the claim: `scripts/work_dot_bat/plan-claim.sh release <subtask_path>`.
4. If the user agrees the task is completed:
   - Mark it `"Done"` in the subtask document and via `scripts/work_dot_bat/plan-status.sh set <subtask_path> "Done"`.
   - If all subtasks in the folder are completed, set the parent `plan.md` `"Done"` via `scripts/work_dot_bat/plan-status.sh set <parent_plan_path> "Done"`.
   - If they tell you something to fix, don't mark the subtask done until they confirm it's fixed.
