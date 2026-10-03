# DAEMON MODE
Triggered if the prompt contains `DAEMON MODE`:
- Skip STEP ZERO entirely (the daemon manages claims, branches, and status).
- Never run `plan-status.sh set` or `plan-claim.sh`.
- Never commit or push to git.
- If routed to `task-breakdown`: create and register the subtask files without asking for review.
- Finish with exactly one final line:
  - `AUTO_OUTCOME: done`
  - `AUTO_OUTCOME: subtasks`
  - `AUTO_OUTCOME: blocked — <one-line reason>` (plus any explanatory detail above it)

# STEP ZERO — Concurrent-work check
**Skip Step Zero entirely** if running under DAEMON MODE (see above). **Skip steps 3's acquire/release** if `next-subtask-yolo.md` invoked you, or the prompt says a launcher (`work.bat` / `batch_work.js`) already holds the plan/branch claim — that launcher's `finally` owns both the acquire and the release. Still run steps 1–2. Otherwise, once you know the target plan file and before anything else:

1. `scripts/work_dot_bat/plan-claim.sh concurrent-check <plan>` — exit 1 (`BLOCKED`): stop, report every reason line verbatim, do not implement. (A bare `In Progress` / `Ready for Review` in frontmatter is not a stop — `work.bat`, `batch_work.js`, and the Auto Daemon set it mechanically before this run.)
2. `git status --short` — if the plan's own target files (named in its "Technical Requirements" / "Research" sections) already show modified: stop and report.
3. Else acquire: `scripts/work_dot_bat/plan-claim.sh acquire <plan> --owner do-work-yolo` (exit 1 = a peer raced you = stop). Release with `scripts/work_dot_bat/plan-claim.sh release <plan>` when this run reaches a terminal state for the plan (Implementation step 3).

Non-interactive: a held claim or an already-advanced plan is a stop, not a question. Stale claims self-recover (TTL `AGENT_TTL_MS` 4h, `plan-claim.sh list` staleness, `--steal`). Under the Auto Daemon, Step Zero claims go into the worktree's own `.claims/` and the daemon clears them — no release needed on exit.

# SETUP
The user gives a problem description or a file. Given only a description, establish scope before proceeding.

# IMPLEMENTATION
Do not ask for the user's opinion until the work is done, unless an instruction says to.

1. Read `agent_docs\00_start_here.md` for relevant documentation (and any matching convention doc in `agent_docs\deep_dives\` before writing code).
2. Route by problem size and announce your choice:
   - small → `agent_docs\instructions\high_trust\red-green-refactor-iteration.md`
   - medium → `agent_docs\instructions\high_trust\small-enhancement-tdd.md`
   - large → `agent_docs\instructions\high_trust\task-breakdown.md` (create the plan files, then stop — do not implement)
3. When implementation and tests are complete: under DAEMON MODE, omit plan-status.sh and plan-claim.sh and output `AUTO_OUTCOME: done`. Otherwise: `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that). If this run acquired the claim in Step Zero, release it: `scripts/work_dot_bat/plan-claim.sh release <plan>`.
   - If a behavior could not be tested and extraction to make it observable is not viable: follow the non-interactive branch of `agent_docs\instructions\testability_escalation.md` — set the plan to `"Blocked - User Input Required"` via `scripts/work_dot_bat/plan-status.sh set <path> "Blocked - User Input Required"`, add a `## Blocked — Needs User Input` section, register a suggestion plan, and release the claim. Never set `Ready for Review` on the strength of a source-text assertion.
