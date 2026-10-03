Follow these steps in order. Ask the user's permission before moving to the next step — this applies to any list of actions you read, not just the ones below.

0. **Step zero — check for concurrent work.** Run the same checks as `agent_docs\instructions\high_trust\do-work-yolo.md`'s Step Zero (`plan-claim.sh concurrent-check <plan>`, then `git status --short` on the plan's target files, then `plan-claim.sh acquire <plan> --owner do-work`), with one trust change: on a `BLOCKED` result, a modified target file, or a raced `acquire`, tell the user every reason line verbatim and ask whether to proceed anyway, pick something else, or wait — don't decide yourself. Release via `scripts/work_dot_bat/plan-claim.sh release <plan>` once the plan reaches a terminal state or the user ends the session.

1. Ask the user what problem to solve, unless already specified.
2. After understanding the problem, read `agent_docs\00_start_here.md` to find relevant documentation (and any matching convention doc in `agent_docs\deep_dives\` before writing code).
3. Use the `AskUserQuestion` tool to tell the user which strategy you plan to use, offering the alternatives as options:
   - small → `agent_docs\instructions\low_trust\red-green-refactor-iteration.md`
   - medium → `agent_docs\instructions\low_trust\small-enhancement-tdd.md`
   - large → `agent_docs\instructions\low_trust\task-breakdown.md`
