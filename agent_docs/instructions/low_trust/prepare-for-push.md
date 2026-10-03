- Run each step below. If any fails, fix it with `agent_docs\instructions\high_trust\do-work-yolo.md` (the high_trust method here is by design). Do not continue or re-run on failure — the user must invoke this skill again.
- Tell the user which commands you run and whether each passed or failed.
- You will not commit or push — the user does that (also a hard permission deny in `.claude\settings.json`).
- If the invocation states the build and test steps (3-6) already ran and passed externally (e.g. via work_dot_bat's prepare_for_push), skip them and go straight to steps 1, 2, 7, 8.

1. Check documentation — see `agent_docs\instructions\low_trust\documentation-sync-before-push.md`.
2. Scan plan files in the changeset (`git diff --name-only origin/main`, plus uncommitted/committed plan changes) for unaddressed recommendations. For each, create `agent_docs/plans/agent_suggestions/<name>.md` and register it: `scripts/work_dot_bat/plan-status.sh add plans/agent_suggestions/<name>.md`.
3. Build and static checks: (none configured)
4. Light tests: (none configured)
5. Heavy tests: (none configured)
6. Re-run any step above that a later fix may have invalidated.
7. Before setting any plan `Ready for Review`, confirm its stated deliverable exists. If not, leave it `In Progress` — never `Cancelled` (the human's call only).
8. Verify `Done` / `Ready for Review` plans: run `scripts/work_dot_bat/plan-status.sh list-changed-done` for plan files modified/added vs `origin/main` marked `Done` or `Ready for Review`. For each, confirm (a) `git diff origin/main` shows implementation code actually changed for it (flag a non-documentation plan with no code), (b) all stated deliverables exist on disk, (c) any commands in its `## Verification Plan` pass and hit target metrics. If any fails, the work is NOT done: revert via `scripts/work_dot_bat/plan-status.sh set <path> "In Progress"` and report specific findings. Do not let push verification claim success while unfulfilled plans are marked `Done`.
