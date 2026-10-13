---
status: Done
subtask_of: plans/october_2026/remove_claims/plan.md
---

# Launcher, session tools and checks without claims

## Goal
`work`, the task menu, session analysis and `run-checks` no longer read or write claims. The menu shows no "claimed" markers and `run-checks` has no claim step.

## Context
Part of `plan.md`; independent of subtask 01. Code: `src/internal/work` (`launch.go` `WithClaim`, `app.go` `ClaimEnv`/`claims()`, `new.go:31`, `menuopts.go`), `src/internal/batch/batch.go` (`WithClaimRelease`), `src/internal/sessiontools/sessiontools.go:166`, `src/internal/checks/runchecks.go:166`, `src/internal/planstatus/cli.go` and `tasks.go` (claim fields), `src/internal/agentsession/analysis.go`, `src/internal/immediate/immediate.go`, `src/cmd/work_dot_bat/main.go:170-178`.

## Requirements
1. No `work`-side or `batch` code references `planclaims`, `WithClaim`, `WithClaimRelease`, or `ClaimEnv`.
2. Task listing/menu output has no claim annotations.
3. `run-checks` no longer runs the claim check.
4. Update affected tests; delete tests that only exercised claim behavior.

## Implementation Plan
Remove dead helpers first (`WithClaim`, `WithClaimRelease`, `claims()`), then the plumbing and display, then the check step. Remove `ListTaskClaims` use from `planstatus` and its callers.

## Testing Plan
- Menu test: with a claim file on a leaf plan, the rendered menu line is identical to the same plan with no claim file.
- `run-checks` test: the step list printed contains no claim step.
- Session-tools test: analysis output for a repo with claim files matches output without them.

## Verification Plan
- `go test ./src/internal/work/... ./src/internal/batch/... ./src/internal/sessiontools/... ./src/internal/checks/... ./src/internal/planstatus/...` passes.
- `go build ./...` passes.
