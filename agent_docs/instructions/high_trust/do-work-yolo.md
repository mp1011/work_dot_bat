# DAEMON MODE
Triggered if the prompt contains `DAEMON MODE`:
- Never run `plan-status.sh set`.
- Never commit or push to git.
- If routed to `task-breakdown`: create and register the subtask files without asking for review.
- Finish with exactly one final line:
  - `AUTO_OUTCOME: done`
  - `AUTO_OUTCOME: subtasks`
  - `AUTO_OUTCOME: blocked — <one-line reason>` (plus any explanatory detail above it)

# SETUP
The user gives a problem description or a file. Given only a description, establish scope before proceeding.

# IMPLEMENTATION
Do not ask for the user's opinion until the work is done, unless an instruction says to.

1. Read `agent_docs\00_start_here.md` for relevant documentation (and any matching convention doc in `agent_docs\deep_dives\` before writing code).
2. Route by problem size and announce your choice:
   - small → `agent_docs\instructions\high_trust\red-green-refactor-iteration.md`
   - medium → `agent_docs\instructions\high_trust\small-enhancement-tdd.md`
   - large → `agent_docs\instructions\high_trust\task-breakdown.md` (create the plan files, then stop — do not implement)
3. When implementation and tests are complete: under DAEMON MODE, omit plan-status.sh and output `AUTO_OUTCOME: done`. Otherwise: `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that).
   - If a behavior could not be tested and extraction to make it observable is not viable: follow the non-interactive branch of `agent_docs\instructions\testability_escalation.md` — set the plan to `"Blocked - User Input Required"` via `scripts/work_dot_bat/plan-status.sh set <path> "Blocked - User Input Required"`, add a `## Blocked — Needs User Input` section, and register a suggestion plan. Never set `Ready for Review` on the strength of a source-text assertion.
