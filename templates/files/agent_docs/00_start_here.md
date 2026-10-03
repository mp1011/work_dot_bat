# WELCOME TO THE {{project_name}} AGENT DOCS

<!-- TODO(agent): replace this paragraph with one or two sentences on what {{project_name}} is and who the docs are for. Keep this file short: agents read it at the start of every session. -->

Agents: read the **Critical Rules** and the **Session Startup Protocol** below, then route your task through the **Task Quick Reference** table. Read the specialized guides and deep dives strictly on demand, never as onboarding.

> **CRITICAL RULES FOR ALL AI AGENTS**:
> 1. **Never start long-running servers.** Starting and managing the dev server ({{dev_server}}) is **always the user's responsibility**.
> 2. **Never commit or push.** Git commits and pushes are made by the user, by hand.
> 3. **Keep plan status in sync.** Every plan file under `agent_docs/plans/` carries its status in a YAML frontmatter block (`---\nstatus: <value>\n---`) at the top. Always change it through `scripts/work_dot_bat/plan-status.sh` — never hand-edit a plan file's frontmatter block.
>    - Starting work on a plan: `scripts/work_dot_bat/plan-status.sh set <path> "In Progress"`.
>    - You believe the work is complete: `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that).
>    - The user agrees it is complete: `scripts/work_dot_bat/plan-status.sh set <path> "Done"`.
>    - Intentionally not finished, declined or abandoned: flag it to the user, who runs `scripts/work_dot_bat/plan-status.sh set <path> "Cancelled" --human`.
>    - Needs a human decision to proceed: `scripts/work_dot_bat/plan-status.sh set <path> "Blocked - User Input Required"`.
>    - Creating a new plan file under `agent_docs/plans/`: `scripts/work_dot_bat/plan-status.sh add <path>` (lands as `"Not Started"`). Start from `{{plan_template}}`.
>    - Untracked recommendations found in plan files: write each to `agent_docs/plans/agent_suggestions/<name>.md` and register it with `scripts/work_dot_bat/plan-status.sh add <path>`.
> 4. **Check for concurrent work before starting a plan**: `scripts/work_dot_bat/plan-claim.sh check <plan>` and `scripts/work_dot_bat/plan-status.sh list-tasks`.

## Session Startup Protocol (Agents)

Start every session by reading **only**:

1. The **Critical Rules** above, plus the safety rules in `CLAUDE.md` / `GEMINI.md`.
2. [`01_overview.md`](01_overview.md) — what the project is, in a paragraph.
3. The **Task Quick Reference** table below — find your task's row, read the one or two docs it names, and start.

### Top-Level Repository Layout

<!-- TODO(agent): one bullet per top-level folder that matters, e.g. `src/` — what lives there. Keep it to ~6 lines; the full file listing is in {{project_structure_doc}}. -->

### Task Quick Reference

| Task | Read |
|---|---|
| Small change or bug fix | `agent_docs/instructions/low_trust/do-work.md` (interactive) or `high_trust/do-work-yolo.md` (unattended) |
| Break a large task into subtasks | `agent_docs/instructions/high_trust/task-breakdown.md` |
| Work the next subtask of a plan | `agent_docs/instructions/high_trust/next-subtask-yolo.md` |
| Break up an oversized file | `agent_docs/instructions/high_trust/monolith-breakup.md` |
| A behavior is hard to test | `agent_docs/instructions/testability_escalation.md` |
| Before pushing | `agent_docs/instructions/low_trust/prepare-for-push.md` |
| Assess whether a plan is really done | `agent_docs/instructions/high_trust/assess-done.md` |
| Project health review | `agent_docs/instructions/high_trust/project-health-assessment.md` |
| Where is a file / what does it do? | `{{project_structure_doc}}` |

<!-- TODO(agent): add a row for each guide in agent_docs/ (architecture, testing, CI/CD, domain notes) as they are written. -->

## How to run things

- Install: {{install}}
- Build and static checks: {{build_commands}}
- Light tests: {{light_test}}
- Heavy tests: {{heavy_test}}
- Mechanical gate (what the pre-push hook and CI run): `scripts/work_dot_bat/run-checks.sh`
