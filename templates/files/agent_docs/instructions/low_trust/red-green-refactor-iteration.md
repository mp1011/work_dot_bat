# RED / GREEN / REFACTOR ITERATION (low-trust)

Follow `agent_docs\instructions\high_trust\red-green-refactor-iteration.md`, with these trust changes:

- Do not move to the next step without the user's permission.
- Step 1: if still unsure after reading `00_start_here.md`, ask the user.
- Step 2: ask the user for review after writing the failing test, before continuing.
- Step 4: ask permission before moving on, even once the test passes.
- Step 6: ask the user before adding the heavy (end-to-end) test.
