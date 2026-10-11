---
status: Not Started
---

# Investigate: how `plan-status.sh` resolves plan paths

## Goal
Establish, from the source and by experiment, exactly which path forms `plan-status.sh` (`work_dot_bat plan-status`) accepts, and decide whether the current behavior needs a fix or only documentation.

## Context
- Observed: `plan-status.sh set agent_docs/plans/october_2026/immediate_inbox/plan.md "Done"` failed with `No entry found for "agent_docs/plans/..."`, while `plan-status.sh set plans/october_2026/immediate_inbox/plan.md "Done"` succeeded. Nothing was modified by the failed call.
- Working hypothesis (unverified): paths are keyed relative to `agent_docs/`. `src/internal/planstatus/frontmatter.go:3` and `scan.go:42` mention "agent_docs-relative" paths, and `subtask_of` frontmatter uses the same form.
- The script header (`.tools/plan-status.sh`) documents `<path>` without saying what it is relative to. `CLAUDE.md` and `agent_docs/00_start_here.md` direct agents to use this script, so a wrong guess costs a failed call at best.
- Unknown: whether other subcommands (`add`, `add-subtask`, `set-verification`, `-parent`) resolve paths the same way, and whether absolute paths, `./`-prefixed paths, Windows backslashes, or paths run from a worktree or subdirectory are accepted.

## Requirements
1. Read the path-handling code for every subcommand that takes a path and document the exact resolution rule, including how the lookup key is built and where "No entry found" is raised.
2. Test each path form against a scratch plan file: repo-root-relative, `agent_docs/`-relative, absolute, `./`-prefixed, backslash separators, and invocation from a subdirectory and from a worktree.
3. Report which forms succeed, which fail, and whether any form silently modifies the wrong file.
4. Recommend one of: (a) accept repo-root-relative and absolute paths by normalizing them to the `agent_docs/`-relative key; (b) keep the behavior and improve the error message to name the expected form; (c) documentation only.

## Implementation Plan
- Trace `plan-status` argument handling in `src/internal/planstatus/commands.go` and the entry lookup behind "No entry found".
- Build a table of path forms vs. subcommands from the experiments.
- Write findings into this file's Context section; do not change code in this plan.
- If a fix is recommended, create a follow-up plan rather than implementing here.

## Testing Plan
- No code changes, so no new tests. If a follow-up fix is planned, it must include a table-driven test over the path forms found here.

## Verification Plan
- The findings table covers every path-taking subcommand and every form listed in Requirement 2.
- The recommendation is justified by the findings and names the exact files the fix would touch.
