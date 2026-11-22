---
status: Done
---

# `run-checks` resolves project config from the main checkout in a worktree

## Goal
`scripts/work_dot_bat/run-checks.sh` works in any git worktree of a project registered in `projects.json`, with no per-worktree `work_dot_bat.config.json`. Daemon runs can then run the full gate where they write code, instead of merging ungated work to `main`.

## Context
- Reported by the ExploreGame project (`BackStage` in `projects.json`, path `C:\Users\mwpas\source\repos\ExploreGame`). Agents in a sibling worktree get `No work_dot_bat.config.json in <worktree>`; the main checkout works. Reproduced with a scratch `git worktree add ../ExploreGame_scratch HEAD`.
- Cause: `internal/checks/runchecks.go`, `LoadConfig(root)` falls back to `projectconfig.ForProject(root, root)`, passing the worktree root as both `treeRoot` and `projectRoot`. `ForProject` matches registry entries by path (`projects.FindProjectByPath`), so a worktree path never matches the main-checkout entry. The legacy `<root>/work_dot_bat.config.json` fallback is absent in projects that have consolidated into `projects.json` (see `deep_dives/consolidated_project_configuration.md`).
- The other call site (`runchecks.go` ~line 602, the `RunAutomatedTests` path) already passes `cwd` and `projectRoot` separately, so it is likely correct; confirm.
- `repo.FindMainRepoRoot(startDir)` (`internal/repo/repo.go`) already resolves a worktree to its main checkout via the `.git` file.
- The env var `WORK_DOT_BAT_PROJECT_ROOT` is not consulted by `LoadConfig`; decide whether it should take precedence over `FindMainRepoRoot`.

## Requirements
1. `LoadConfig(root)` looks up the registry with the main checkout root: `ForProject(root, repo.FindMainRepoRoot(root))`.
2. In a worktree, check commands still run with the worktree as their working directory (config is resolved from the main root, but commands execute in the worktree).
3. Behavior in the main checkout is unchanged, including when a legacy `work_dot_bat.config.json` exists in the tree (the tree's own file still wins over the registry, as today).
4. Audit other callers that load project config from a worktree path (grep `ForProject(`, `LoadConfig(`, `projectconfig.Load(`) and fix any with the same root/projectRoot confusion.

## Implementation Plan
Change `LoadConfig` to derive the project root with `repo.FindMainRepoRoot`. Check that `checks` can import `repo` without a cycle; if not, pass the project root in as a parameter from the caller. Then do the audit in Requirement 4.

## Testing Plan
Unit tests in `internal/checks` (temp dirs, temp registry file via the registry path override the existing `projectconfig` tests use):
- Worktree-shaped dir (`.git` file containing `gitdir: <main>/.git/worktrees/x`), `<main>` registered with `checks` configured, no config file in the worktree: `LoadConfig(worktree)` returns the registered config. Observed via the returned `Config.Checks`.
- Same setup with an unregistered main: returns the "No work_dot_bat.config.json" error.
- Plain checkout (`.git` directory) registered in the registry: unchanged result.
- Worktree containing its own `work_dot_bat.config.json`: that file wins.

## Verification Plan
- `go test ./...` in `src`.
- In the ExploreGame repo: `git worktree add ../ExploreGame_scratch HEAD`, then `scripts/work_dot_bat/run-checks.sh --skip-tests` there runs the plan-drift, monolith, duplication and skills checks instead of erroring. Remove the worktree afterwards.
- Rebuild `dist/` per the repo's build instructions so the installed binary has the fix.
