## SUMMARY
Break a high-level or composite task into separate, actionable subtasks for other agents to implement. Do not write or run code in this phase.

## PHASE 1: Plan File
The user gives you a plan file with a high-level goal and context.

## PHASE 2: Task Breakdown
1. Break each task into focused, actionable, testable subtasks:
   - no dedicated testing subtask
   - every subtask needs a user-noticeable component (no backend-only tasks)
   - if a subtask is more than one workday for a skilled engineer, give it child subtasks by the same criteria
2. Ask the user to review the subtasks before proceeding.

## PHASE 3: Individual Plan Files
1. For each subtask, create a markdown file in this folder from `{{plan_template}}`, filling out each section. The Testing Plan must name observable behavior — "Verify file X contains Y" is not a test plan (see `agent_docs\instructions\testability_escalation.md`). Register each: `scripts/work_dot_bat/plan-status.sh add-subtask <path> -parent <path to parent>`.
2. Ask the user to review each plan file.

Then stop — the user spins separate agents per task.
