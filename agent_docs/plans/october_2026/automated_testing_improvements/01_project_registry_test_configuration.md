---
status: Done
subtask_of: plans/october_2026/automated_testing_improvements/plan.md
---

# Project Registry Test Layer Configuration in projects.json

## Goal
Projects registered in `projects.json` can specify commands for four distinct test tiers (`ultra_light`, `light`, `heavy`, `ultra_heavy`). The registry validator checks these command definitions, preserving them across registry loads and saves, and `projects.example.json` provides a clear template for users.

## Context
Part of `agent_docs/plans/october_2026/automated_testing_improvements/plan.md`.
Currently, `internal/projects/projects.go` defines `Project` with only `Name` and `Path`. Repo-specific test commands previously resided in each repo's `work_dot_bat.config.json` under `lightTest` and `heavyTest`.
The new structure moves test command definitions directly under a `tests` object on each project in `projects.json`.
Relevant files: `internal/projects/projects.go`, `internal/projects/projects_test.go`, `projects.example.json`.

## Requirements
1. Define a `TestsConfig` structure on `projects.Project` containing `UltraLight`, `Light`, `Heavy`, and `UltraHeavy` command definitions (string slices representing `[cmd, ...args]`).
2. Extend `ValidateRegistry` to validate the `tests` object and each layer's command format when present (must be array of strings, command name must not be empty).
3. Ensure `LoadRegistry` and `SaveRegistry` round-trip `tests` definitions without data loss or formatting corruption.
4. Update `projects.example.json` with an annotated example showing how `tests` are configured with `ultra_light`, `light`, `heavy`, and `ultra_heavy`.

## Implementation Plan
Add `TestsConfig` struct and fields to `projects.Project`. Update `ValidateRegistry` to parse and validate `tests` on each entry. Add unit tests in `internal/projects/projects_test.go` covering valid configurations, empty test blocks, and malformed command entries (e.g. non-string elements, empty command names). Update `projects.example.json`.

## Testing Plan
- Valid test config: A registry with valid `ultra_light`, `light`, `heavy`, and `ultra_heavy` string arrays parses with 0 validation errors, and `FindProject` returns the populated `TestsConfig`.
- Malformed test entries: A registry containing a non-object `tests` field, non-array layers, or arrays containing non-strings returns descriptive validation error messages identifying the offending project and field.
- Round-trip save: Saving and reloading a registry with `tests` preserves all layer commands exactly.

## Verification Plan
`go test ./internal/projects/... -run TestRegistry` passes cleanly.
