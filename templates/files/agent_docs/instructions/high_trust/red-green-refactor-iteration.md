# RED / GREEN / REFACTOR ITERATION
Solve a small coding problem test-first. Write exactly ONE test — isolate a single behavior, not full coverage. Use your judgement; do not ask for the user's opinion.

1. Read `agent_docs\00_start_here.md` for context on what to change.
2. Write one test whose name states the behavior. Follow any testing guidance in `agent_docs\` (communicate intent inside the test, helpers for mechanisms). First follow `agent_docs\instructions\testability_escalation.md` to name the observable; if no assertion can distinguish implemented from unimplemented behavior, escalate per its non-interactive branch instead of proceeding. Force a failure (e.g. `expect(true).toBe(false)`).
3. Make the test fail for real — only because the feature is unimplemented. Scaffolding code to make the failure meaningful is allowed; implementing the feature is not. Run it to confirm it fails for the right reason.
4. Implement the minimum to pass (read any matching convention doc in `agent_docs\deep_dives\` first). Run it to confirm.
5. Refactor the implementation if it isn't maintainable — never the test.
6. If the change is visible to the project's heavy tests, add/update the matching heavy test and run {{heavy_test}}. Skip otherwise.
7. `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that).
