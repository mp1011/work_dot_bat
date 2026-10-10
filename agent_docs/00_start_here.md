# WELCOME TO THE work_dot_bat AGENT DOCS

<!-- TODO(agent): replace this paragraph with one or two sentences on what work_dot_bat is and who the docs are for. Keep this file short: agents read it at the start of every session. -->

Agents: read the **Critical Rules** and the **Session Startup Protocol** below, then route your task through the **Task Quick Reference** table. Read the specialized guides and deep dives strictly on demand, never as onboarding.

> **CRITICAL RULES FOR ALL AI AGENTS**:
> 1. **Never start long-running servers.** Starting and managing the dev server ((none configured)) is **always the user's responsibility**.
> 2. **Never commit or push.** Git commits and pushes are made by the user, by hand.
> 3. **Keep plan status in sync.** Every plan file under `agent_docs/plans/` carries its status in a YAML frontmatter block (`---\nstatus: <value>\n---`) at the top. Always change it through `.tools/plan-status.sh` — never hand-edit a plan file's frontmatter block.
>    - Starting work on a plan: `.tools/plan-status.sh set <path> "In Progress"`.
>    - You believe the work is complete: `.tools/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that).
>    - The user agrees it is complete: `.tools/plan-status.sh set <path> "Done"`.
>    - Intentionally not finished, declined or abandoned: flag it to the user, who runs `.tools/plan-status.sh set <path> "Cancelled" --human`.
>    - Needs a human decision to proceed: `.tools/plan-status.sh set <path> "Blocked - User Input Required"`.
>    - Creating a new plan file under `agent_docs/plans/`: `.tools/plan-status.sh add <path>` (lands as `"Not Started"`). Start from `agent_docs/plan_template.md`.
>    - Untracked recommendations found in plan files: write each to `agent_docs/plans/agent_suggestions/<name>.md` and register it with `.tools/plan-status.sh add <path>`.
> 4. **Check for concurrent work before starting a plan**: `.tools/plan-claim.sh check <plan>` and `.tools/plan-status.sh list-tasks`.

## Session Startup Protocol (Agents)

Start every session by reading **only**:

1. The **Critical Rules** above, plus the safety rules in `CLAUDE.md` / `GEMINI.md`.
2. [`01_overview.md`](01_overview.md) — what the project is, in a paragraph.
3. The **Task Quick Reference** table below — find your task's row, read the one or two docs it names, and start.

### Top-Level Repository Layout

- `.agents/` — Agent skills
- `.claude/` — Claude configuration and skills
- `.tools/` — Runnable scripts and tools (both user and agent commands)
- `.work_dot_bat/` — Internal runtime state (claims, logs, session ledger)
- `agent_docs/` — Agent documentation and plans
- `dist/` — Compiled release binaries (gitignored)
- `src/` — Go codebase (`src/cmd/`, `src/internal/`, `src/vendor/`)
- `templates/` — Repository template files

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
| Where is a file / what does it do? | `agent_docs/02_project_structure.md` |

<!-- TODO(agent): add a row for each guide in agent_docs/ (architecture, testing, CI/CD, domain notes) as they are written. -->

## How to run things

- Install: (none: the only dependency is the Go toolchain)
- Build and static checks: `go build ./...`, `go vet ./...`
- Light tests: `go test ./...`
- Heavy tests: (none configured)
- Release builds: `.tools/build-release.sh` (all six OS/arch targets into `dist/`), then `.tools/smoke.sh dist/<binary>`
- Mechanical gate (what the pre-push hook and CI run): `.tools/run-checks.sh`

## Project Configuration

Project configuration (build commands, dev server, install command, test suites, worktree file copies, and structure doc paths) is consolidated directly in `projects.json` (see `projects.example.json` for reference), with backwards compatibility for legacy `work_dot_bat.config.json`.
