## TASK BREAKDOWN (low-trust)

Follow `agent_docs\instructions\high_trust\task-breakdown.md`, with these trust changes:

- Tell the user what phase you are in and don't advance without permission.
- Before Phase 1's "Plan File": the user names the plan folder; create it under `agent_docs/plans/` if it doesn't exist, create a `plan.md` with sections "Overall Goal" and "Tasks", and register it with `scripts/work_dot_bat/plan-status.sh add <path>`.
- High-level planning: ask the user for the overall goal, then for tasks one at a time (adding each as a subsection under "Tasks") until they say they are done.
- Questions phase: ask the user questions about each task and write their answers under each task.
