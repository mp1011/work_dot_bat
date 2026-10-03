# GOAL
Given a plan file (auto-written by `scripts/work_dot_bat/check-monolith.sh check` under `agent_docs/plans/<month>_<year>/monolith_breakup/`, or a bare path to an oversized file) naming a TS/TSX file over the monolith line threshold, drive it to a direct fix or a tracked breakdown — never leave the plan with an empty Implementation Plan.

## STEPS
Use your judgement; don't ask for the user's opinion unless genuinely blocked.

1. Read the plan file — it names the offending file and its flagged line count.
2. Re-check whether the file is still over threshold (`scripts/work_dot_bat/check-monolith.sh check`, or recount lines). If already under, set the plan `"Ready for Review"` and stop.
3. Read the file in full and identify its distinct responsibilities. Look for a precedent split before inventing a new shape — grep `agent_docs/project_health/*.md` and `agent_docs/deep_dives/`.
4. Fill in the plan's `# Research`, `# Implementation Plan`, and `# Testing Plan` with the concrete split — which pieces move to which new files, what stays, how existing tests follow the moved code.
5. Route by the size of the split (same criteria as `agent_docs/instructions/high_trust/do-work-yolo.md`):
   - **Small** (one clean extraction, one session) → follow `red-green-refactor-iteration.md` or `small-enhancement-tdd.md` and implement now.
   - **Large** (several independent extractions) → follow `task-breakdown.md`: write child subtask plan files and STOP.
6. After a split lands, run `scripts/work_dot_bat/check-monolith.sh baseline` to tighten the ratchet file — the split file drops out once back under threshold, which doubles as verification. `baseline` only shrinks; it refuses (exit 1) to add a path or raise a value. If you decide a file should stay whole, record that with `scripts/work_dot_bat/check-monolith.sh baseline --accept <path> --reason "<why>"` — the reason lands in the baseline file's `reasons` map and keeps `check` from reopening this plan if the file grows again.
7. Set the plan `"Ready for Review"` via `scripts/work_dot_bat/plan-status.sh set <path> "Ready for Review"`. Never set `"Done"` directly.

## Non-goals
- This instruction doesn't set the threshold or which files are scanned — that's `scripts/work_dot_bat/check-monolith.sh`. Don't hand-edit `agent_docs/monolith_baseline.json`; the `baseline` command maintains it.
- Don't split for its own sake. If a long file is genuinely cohesive (a big lookup table, one well-tested algorithm), say so in Research and prefer `baseline --accept <path> --reason "<why>"` over a forced split.
