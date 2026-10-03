# GOAL
Make a small enhancement using Test-Driven Development. Work the phases in order; do not ask for the user's opinion until the feature is ready for review. Surface phase progress so an unattended run is followable.

1. **Plan file** — write the problem into a file in `agent_docs\plans`; register it: `scripts/work_dot_bat/plan-status.sh add <path>`.
2. **Clarity check** — scaffold with `agent_docs\00_start_here.md`; use your recommended approach for any open question.
3. **Exploration** — explore the codebase and docs. Add findings to a "Research" section; list relevant files under "Context" (only what is undocumented or needs attention). Fill in "Implementation Plan" and "Testing Plan".
4. **Scaffolding** — write only enough code to make testing possible; no business logic. New methods declare their signature and throw "not implemented".
5. **Failing tests** — follow `agent_docs\instructions\testability_escalation.md` to name the observable and any testing guidance in `agent_docs\` for test intent and helper conventions; if you cannot and extraction is not viable, escalate per its non-interactive branch instead of writing a source-text assertion. Write tests for the desired behavior; confirm they fail, and fail precisely because the feature is unimplemented.
6. **Implementation** — minimum code to make the tests pass. Read any matching convention doc in `agent_docs\deep_dives\` first.
7. **Manual testing** — self-verify end-to-end with the `verify` skill (or `run` if nothing is verifiable yet), then ask the user to review. Any issue → return to phase 5 with a failing test.
8. **Heavy-test regression** — lock in the verified behavior with a heavy test (no red phase) and run {{heavy_test}} for the affected spec. If it is not covered by the heavy suite, state why and skip.
9. **Documentation** — write learnings into a file under `agent_docs/deep_dives`. Run `scripts/work_dot_bat/update-project-structure.sh` for any new files, then fill in `TODO: summarize` placeholders (never hand-edit the generated tree). Scan the plan and research notes for unimplemented recommendations; write each to `agent_docs/plans/agent_suggestions/<name>.md` and register it: `scripts/work_dot_bat/plan-status.sh add plans/agent_suggestions/<name>.md`.
10. **Status update** — `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"` (never `"Done"` — only the user confirms that).
