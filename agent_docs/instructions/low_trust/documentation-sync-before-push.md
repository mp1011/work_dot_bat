# GOAL
Review the files changed since the last push and verify the agent documentation is still accurate.

# STEPS
- Use git to find files changed locally but not yet pushed to origin.
- From the files modified, work out which documentation files may need updating (files in `agent_docs`, excluding `agent_docs/plans`).
- Compare those docs against the actual code; determine if anything is wrong or missing.
- Run `scripts/work_dot_bat/update-project-structure.sh --check` as a drift gate for `agent_docs/02_project_structure.md` — it exits non-zero on file-set drift *and* while any `TODO: summarize` placeholder remains. If it reports drift, run `scripts/work_dot_bat/update-project-structure.sh` (never hand-edit the generated tree), then fill in every `TODO: summarize` placeholder (read the file first).
- Check any plan files in the changeset for unaddressed recommendations. For each, write `agent_docs/plans/agent_suggestions/<name>.md` and register it: `scripts/work_dot_bat/plan-status.sh add plans/agent_suggestions/<name>.md`.
- Show the user your findings, then use `AskUserQuestion` to ask permission before modifying documentation.
