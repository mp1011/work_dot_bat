---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Go task picker menu

## Goal
The interactive "Step 1" task menu behaves as it does today: incomplete tasks listed in the same order and grouping with the same line format, plus write-in, paste-path, parked-suggestions and skip/quit options. `create_worktree`'s plan mode (subtask 03) uses this picker instead of its placeholder list.

## Context
Part of `04_interactive_work.md`; depends on 02a–c and 03. Source: `src/lib/work-common.mjs` — `selectTask`, `getRunnableAndGroupedTasks`, `getParkedSuggestions`, `summarizeSuggestion`, `formatSuggestionList`, `promptSuggestionSelection`, `formatTaskLine`, `colorizeStatus`, `taskHasSubtasks`, `isAutoTask`, `isPlanMarkedDoneInMain`, `resolvePlanPath`, `processPastedPlanPath`, `getPlanFolders`, `handleWriteInTask` and the prompt helpers; tests in `work-common.test.ts`. `ensurePlanFrontmatter` already exists in `internal/planstatus`. Input and output go through an injected prompt interface so the menu is testable without a TTY.

## Requirements
1. Same task ordering (priority, then filename), subtask grouping, numbering, colorized status (only on a TTY), claim and worktree annotations.
2. Options: number, `w` write-in (folder, filename, problem description, creating a registered new plan file), `p` paste a plan path, `v` view parked suggestions, `0`/`s` skip, `q` quit.
3. Selection returns the chosen plan (or none) to the caller; `create_worktree` plan mode switches to it.

## Implementation Plan
New package `internal/taskmenu`: pure list-building and line formatting first, then the prompt loop over an injected `Prompter`.

## Testing Plan
- A temp plans tree with a parent, two prioritized subtasks, a done plan and two parked suggestions renders a golden menu (indices, grouping, annotations, the `v` line with the count).
- Scripted input `v`, `1`, then back out returns the suggestion; `w` with folder, filename and description creates the file with `Not Started` frontmatter; `p` with a path outside the plans tree is rejected with the Node message.
- `0` and `q` return no selection without touching any file.

## Verification Plan
`go test ./...`; diff the rendered menu for this repo against the Node launcher's menu text by running both with piped `0`.
