# GOAL
Identify what is healthy and unhealthy in the current state of the project. Consult past health assessments in `agent_docs/project_health/` for guidelines, plus the rules below.

## Specific Areas to Measure
1. Technical debt — monoliths (`scripts/work_dot_bat/check-monolith.sh`), excessive repetition (`check-duplication.sh`, `check-structural-duplication.sh`), deviations from best practices.
2. Plan discipline — continually ignored plans (`scripts/work_dot_bat/plan-status.sh list-tasks`, plans stuck `In Progress` or `Not Started`).
3. Test health — flaky or slow tests, untested modules, source-text assertions (`check-source-assertions.sh`).
4. Agentic development technique:
   - Are we wasting tokens on onboarding docs?
   - Are we using agents to their full potential?
   - Do we have proper checks and balances?
5. Add project-specific metrics here as they become relevant (trend lines against prior assessments).

## "Mode" Recommendation
Give your judgement on which development mode would be most fruitful:
1. Rapid Development (trying new ideas)
2. Stabilization (ensure what we have works)
3. UX Tuning (works, but make it feel right)
4. Performance Tuning (works and feels right, make it perform)
5. Tech Debt Paydown (works outside, mess inside?)
6. Agent Harness Review (improve our use of AI)

## Required Output
1. Write the assessment to `agent_docs/project_health/project_health_<YYYY>_<M>_<D>.md`.
2. Create a followup plan folder at `agent_docs/plans/<month>_<year>/<date>_health_followup/` containing a `plan.md` index plus one subtask file per recommendation, each registered with `scripts/work_dot_bat/plan-status.sh add` (lands as `Not Started`).
3. In the next assessment, open by auditing the previous followup folder — report each item's status and treat any still `Not Started` as a finding. Check each item against the recommendation in the prior health report that generated it, not only the plan file's own text (which may have been rewritten to match what was built). Treat a `Done` item whose deliverable does not exist as a finding. Step 3 closes the loop — without it, recommendations live only in a dated report nothing reads again.
