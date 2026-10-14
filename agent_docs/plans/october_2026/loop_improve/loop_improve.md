---
status: Ready for Review
---

# GOAL

Improve the "Loop until Done" menu option to better cover a large, broken-down feature.

# Context

Currently, the option is entirely linear, going through each child subtask in order. This has two flaws:
- it doesn't handle nested plans
- it assumes plans are best done sequentially. This is usually true for immediate children but not always.

# Tasks
1. Add a new agent skill which determines a specific task order for a parent task and writes it to an array in the frontmatter
2. Change the "Loop until Done" task to first check if there is a plan order in the frontmatter
    - if not, the skill from #1 is used to create it
    - the next incomplete task from this array is chosen
    - otherwise, the behavior is the same

# Research

- Frontmatter in `src/internal/planstatus/frontmatter.go` parses YAML frontmatter using regexes. `blocked_by` parsing demonstrates handling of both bracket arrays `[a, b]` and multi-line bullet lists `- a`. We can generalize list parsing for `plan_order`.
- Plan entry scanning in `src/internal/planstatus/scan.go` reads frontmatter into `Entry`. Adding `PlanOrder []string` allows plan entries to carry task order.
- `RunLoopUntilDone` in `src/internal/batch/loop.go` drives loop mode. It currently selects subtasks via `GetEligibleSubtasks(r.Parent, r.GetTasks())`.
- When `plan_order` is present in the parent plan's frontmatter, the loop should pick the first unresolved (`IsSubtaskEligible`) subtask from `plan_order`.
- If `plan_order` is not present, `RunLoopUntilDone` invokes the `/plan-order` skill via `r.RunAgent` once, re-checks frontmatter, and if present picks from `plan_order`, otherwise falling back to `GetEligibleSubtasks`.
- Skills integrity is verified by `CheckSkills` in `src/internal/checks/skills.go`. A new skill `plan-order` must exist in `.agents/skills/plan-order/SKILL.md` and `.claude/skills/plan-order/SKILL.md` (and mirrored in `templates/files/`), pointing to `agent_docs/instructions/high_trust/plan-order.md`.

# Implementation Plan

1. Skill creation:
   - Add `.agents/skills/plan-order/SKILL.md`, `.claude/skills/plan-order/SKILL.md`, and their template mirrors.
   - Add instruction doc `agent_docs/instructions/high_trust/plan-order.md` and template mirror.
   - Update `agent_docs/00_start_here.md` and template mirror.
2. Frontmatter & planstatus support:
   - Add `PlanOrder []string` to `Frontmatter` and `Entry`.
   - Update `ParseFrontmatter` to parse `plan_order` (supporting inline brackets and bullet lists).
   - Add `SetFrontmatterPlanOrder` in `frontmatter.go`.
   - Add `SetPlanOrder` command to `planstatus`.
3. Loop until Done enhancements:
   - Update `batch.RunLoopUntilDone` in `src/internal/batch/loop.go`:
     - Check parent plan for `plan_order` in frontmatter (via `ReadPlan` or `Parent.PlanOrder`).
     - If not present and not yet attempted, run agent with `/plan-order <parentPlanPath>`.
     - Select the next incomplete task from `plan_order` using `IsSubtaskEligible`.
     - When all tasks in `plan_order` are resolved, proceed to parent task.
     - Fall back to `GetEligibleSubtasks` if no `plan_order` exists.
4. Testing:
   - Unit tests in `planstatus_test.go` for `plan_order` parsing and `SetFrontmatterPlanOrder`.
   - Unit tests in `batch_test.go` for `RunLoopUntilDone`:
     - Subtasks selected in `plan_order` sequence rather than default alphabetical/priority sequence.
     - Nested subtasks included in `plan_order` are properly executed.
     - When `plan_order` is missing, `/plan-order` agent is invoked; if agent populates `plan_order`, execution follows it.
     - When `plan_order` is missing and agent does not populate it, execution falls back to default `GetEligibleSubtasks`.
   - Skill check in `checks_test.go` / `run-checks.sh`.

# Testing Plan

- Automated tests:
  - `work_dot_bat/src/internal/planstatus`: test `ParseFrontmatter` with inline `[a, b]` and bullet `- a` `plan_order`; test `SetFrontmatterPlanOrder` replaces or adds `plan_order:`.
  - `work_dot_bat/src/internal/batch`: test `RunLoopUntilDone` with pre-existing `plan_order`, test `RunLoopUntilDone` invoking `/plan-order` when missing, and fallback when `/plan-order` yields no order.
  - `work_dot_bat/src/internal/checks`: test `CheckSkills` verifies `plan-order` in both `.agents` and `.claude`.
  - Full suite `go test ./...`.