---
status: Done
subtask_of: plans/initial_port/plan.md
---

# Go scaffold, launcher, and project registry

## Goal
Running `work_dot_bat <tool> project=<name>` (and `work_dot_bat` with no args for the project menu) works from a single Go binary on Windows, macOS and Linux: it resolves the registered project, loads its config, and dispatches to a tool. Tools not yet ported report a clear "not yet ported" message.

## Context
Part of the Go port in `plan.md`. Port behavior with identical user-visible behavior unless noted; the Node code stays until subtask 09 (cutover) so the two can be compared side by side.
Source: `src/launch.mjs`, `src/lib/projects.mjs`, `project-config.mjs`, `project-menu.mjs`, `project-context.mjs`, `engine-config.mjs`, `main-repo-root.mjs`, `fs-guard.mjs`, `state-paths.mjs`, `projects.example.json`, `work_dot_bat.config.json`, and their `*.test.ts`.
This subtask decides the module path and layout (`cmd/work_dot_bat`, `internal/...`); later subtasks follow it. Keep the `projects.json` / `WORK_DOT_BAT_REGISTRY` / per-repo `work_dot_bat.config.json` formats unchanged.

## Requirements
1. `go.mod` and `cmd/work_dot_bat/main.go` with a subcommand dispatcher and a tool registry that later subtasks plug into.
2. Project registry read/write, `project=<name>` resolution, interactive menu (pick a number, or `p` to paste a repo path and register it).
3. Per-repo config loading, repo-root detection (including linked worktrees), state-path resolution, path guard.
4. `go build ./...` and `go vet ./...` are clean; `go test ./...` runs.

## Implementation Plan
Create the module and dispatcher; port the library packages with table-driven Go tests ported from the matching `*.test.ts` files.

## Testing Plan
- Registry: given a temp registry JSON, `project=foo` resolves to its path; an unknown name returns an error naming the registered projects. Observed via the resolve function and the CLI exit code/stderr.
- Menu: feeding `2\n` / `p\n<path>\n` on stdin selects the 2nd project / registers the path; observed in the registry file afterward.
- Repo root: in a temp git repo with a linked worktree, root detection returns the main repo root from both locations.
- CLI: running an unported tool exits non-zero with "not yet ported".

## Verification Plan
`go test ./...`; `go run ./cmd/work_dot_bat` shows the menu against `projects.example.json`; `go run ./cmd/work_dot_bat nosuchtool` prints a usage error.
