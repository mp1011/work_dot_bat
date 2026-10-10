---
status: Done
subtask_of: plans/october_2026/automated_testing_improvements/plan.md
---

# Consolidate Project Configuration into projects.json

## Goal
All project configuration settings needed by `work_dot_bat` (build commands, dev server, install command, worktree file copies, and documentation paths) are held directly inside `projects.json`, eliminating the need for a separate `work_dot_bat.config.json` while maintaining full backwards compatibility for repos that still have one.

## Context
Part of `agent_docs/plans/october_2026/automated_testing_improvements/plan.md`.
The overall goal states: "Contain all configuration that work.bat needs for a project inside projects.json. Currently we put the test commands in a separate json, but we want all configuration to be available in one place."
Currently `internal/projectconfig/projectconfig.go` reads `<repo>/work_dot_bat.config.json`.
By consolidating project configuration into `projects.json`, each registered project's entire configuration is centralized in one file.
Relevant files: `internal/projects/projects.go`, `internal/projectconfig/projectconfig.go`, `internal/menu/menu.go`, `internal/bootstrap/bootstrap.go`, `internal/worktree/provision.go`.

## Requirements
1. Allow project entries in `projects.json` to optionally specify all repo configuration fields: `build`, `install`, `devServer`, `checks`, `extraChecks`, `worktreeCopyFiles`, `projectStructureDoc`, and `projectStructureDirs`.
2. Update config resolution (`projectconfig.ForProject` and callers) to read configuration directly from the matched project in `projects.json`, falling back to `work_dot_bat.config.json` if absent or not yet migrated.
3. Update interactive project registration in `internal/menu/menu.go` to prompt for and write these configuration values into `projects.json`.
4. Update `bootstrap` so newly bootstrapped repositories register their configuration into `projects.json` rather than generating `work_dot_bat.config.json`.
5. Update docs and examples (`projects.example.json`, `agent_docs/00_start_here.md`) describing the consolidated configuration format.

## Implementation Plan
Extend `projects.Project` to embed or include optional project configuration fields. Update `projectconfig.ForProject` to query `projects.LoadRegistry` first for the project's config block. Refactor `internal/menu/menu.go` project registration flow to save configuration directly into `projects.json`. Update `bootstrap` tool and documentation.

## Testing Plan
- Config resolution precedence: For a project with settings in `projects.json`, `ForProject` returns the values from `projects.json`. For a project without settings in `projects.json` but with a `work_dot_bat.config.json`, it returns the legacy values.
- Menu registration: Registering a new project via the interactive menu writes both the project path and its configuration into `projects.json` without writing a `work_dot_bat.config.json`.
- Worktree provisioning: Worktree provisioning loads `install` and `devServer` from `projects.json` and executes them properly.

## Verification Plan
`go test ./internal/projects/... ./internal/projectconfig/... ./internal/menu/...` passes; `projects.example.json` validates against `ValidateRegistry`.
