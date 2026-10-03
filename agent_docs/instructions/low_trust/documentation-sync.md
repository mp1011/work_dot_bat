# GOAL
Ensure project documentation is accurate. Go through each tier one at a time. For any mistake, show the user the proposed change and use `AskUserQuestion` to let them approve, edit, or skip it before moving to the next tier.

# SYNCING agent_docs/02_project_structure.md
The tree/bullet listing is machine-generated — never hand-add, hand-remove, or reorder file bullets.
- Sync with `scripts/work_dot_bat/update-project-structure.sh` (regenerates the tree from disk, preserves existing summaries).
- Then search for `TODO: summarize` and write a one-line summary for each — what the module is *for*, in the voice of the surrounding entries; read the file first, a filename guess is worse than the honest placeholder. This is the only manual edit permitted in the generated region.
- Run `scripts/work_dot_bat/update-project-structure.sh --check` to confirm the doc is in sync before finishing this tier — it now also exits non-zero while any `TODO: summarize` placeholder remains, not just on file-set drift.

# TIER 1 — Critical
- agent_docs\00_start_here.md
- agent_docs\01_overview.md
- agent_docs/02_project_structure.md

# TIER 2 — Important
- every other top-level guide in `agent_docs\` (the numbered `NN_*.md` files)

# TIER 3 — Special Purpose
- agent_docs\deep_dives (all files)

# IGNORE
Do not assess: fixes, plans, project_health.
