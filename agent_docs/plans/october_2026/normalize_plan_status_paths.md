---
status: Not Started
---

# Normalize plan path resolution across plan-status subcommands

## Goal
Ensure `plan-status.sh` (`work_dot_bat plan-status`) consistently accepts all common path forms—including repo-root-relative (`agent_docs/plans/...`), `./`-prefixed (`./agent_docs/...`, `./plans/...`), Windows backslash, and absolute paths—across all path-taking subcommands (`set`, `set-verification`, `add`, `add-subtask`).

## Context
- Investigation in `agent_docs/plans/october_2026/plan_status_path_resolution.md` revealed that `add` and `add-subtask` normalize paths using `NormalizePlanPath`, but `set`, `set-verification`, `SetBody`, and `ReadEntryStatus` pass the path argument directly into `readFile(root, entryPath)`.
- Consequently, `plan-status.sh set agent_docs/plans/...` fails with `No entry found`, while `add agent_docs/plans/...` succeeds.
- Additionally, `NormalizePlanPath` only strips exact `"agent_docs/"` string prefixes and quotes/backslashes, but does not strip `./` or resolve absolute paths against `c.Root`.

## Requirements
1. Enhance plan path normalization (e.g. `NormalizePlanPath` or a repo-aware resolver) to handle:
   - `agent_docs/`-relative paths (`plans/...`)
   - Repo-root-relative paths (`agent_docs/plans/...`)
   - Dot-slash prefixed paths (`./agent_docs/...`, `./plans/...`)
   - Windows backslash separators (`agent_docs\plans\...`, `plans\...`)
   - Absolute filesystem paths under the repo root
2. Update `Set`, `SetVerification`, `SetBody`, `ReadEntryStatus`, and `ReadBody` in `src/internal/planstatus/commands.go` to normalize the incoming `entryPath`.
3. Update `cli.go` output formatting to report the canonical normalized path consistently.
4. Add table-driven unit tests in `src/internal/planstatus/planstatus_test.go` verifying that all path forms succeed across `add`, `add-subtask`, `set`, and `set-verification`.

## Implementation Plan
- Update `NormalizePlanPath` (or add helper functions in `src/internal/planstatus`) to clean paths, resolve against `root` if absolute, and strip `agent_docs/` cleanly.
- Ensure `commands.go` consistently normalizes before lookup.
- Ensure `cli.go` dispatches normalized paths.

## Testing Plan
- Unit tests in `planstatus_test.go` covering:
  - Table of path representations (repo-relative, docs-relative, `./`-prefixed, absolute, backslashes).
  - Validation across `add`, `add-subtask`, `set`, and `set-verification`.

## Verification Plan
- `go test ./src/internal/planstatus/...` passes.
- `./.tools/run-checks.sh` passes.
