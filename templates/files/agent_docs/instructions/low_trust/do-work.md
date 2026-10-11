Follow these steps in order. Ask the user's permission before moving to the next step — this applies to any list of actions you read, not just the ones below.

1. Ask the user what problem to solve, unless already specified.
2. After understanding the problem, read `agent_docs\00_start_here.md` to find relevant documentation (and any matching convention doc in `agent_docs\deep_dives\` before writing code).
3. Use the `AskUserQuestion` tool to tell the user which strategy you plan to use, offering the alternatives as options:
   - small → `agent_docs\instructions\low_trust\red-green-refactor-iteration.md`
   - medium → `agent_docs\instructions\low_trust\small-enhancement-tdd.md`
   - large → `agent_docs\instructions\low_trust\task-breakdown.md`
