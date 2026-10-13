---
status: Done
subtask_of: plans/october_2026/remove_claims/plan.md
---

# Daemon runs without claims

## Goal
The auto daemon picks the next available task and runs it without acquiring, honoring, or showing claims. A leftover claim file for a plan no longer makes the daemon skip it, and `auto_status` has no claims section.

## Context
Part of `plan.md`. Daemon code: `src/internal/daemon/run.go` (owner/scopes/`ClaimEnv`/`Release`, ~lines 81-117 and 558-563), `plans.go` (`hasLiveClaim`, `Claims` field), `loop.go` (`Claims` func), `cli.go` (`ListClaims`, status output), `agent.go`. Wiring in `src/internal/work/new.go` and `src/cmd/work_dot_bat/main.go:170`. A second daemon on the same repo is explicitly unsupported; do not add a replacement lock.

## Requirements
1. The daemon never calls `planclaims` (acquire, release, list).
2. Plan selection ignores claims: a plan with a pre-existing live claim file is still picked.
3. `auto_status` output no longer lists claims.
4. Remove the now-unused daemon fields/params and update the daemon tests.

## Implementation Plan
Delete the claim acquisition/release around each run, the `hasLiveClaim` filter and the `Claims` plumbing, and the status rendering. Let the compiler find the remaining references. Leave `planclaims` itself for subtask 03.

## Testing Plan
- Daemon test: write a live claim file for the only eligible plan; run one daemon iteration; observe the agent is launched for that plan (previously skipped).
- Daemon test: after a run, no file exists under `.work_dot_bat/claims`.
- `auto_status` test: with a claim file present, output contains no claim lines.

## Verification Plan
- `go test ./src/internal/daemon/...` passes.
- `go build ./...` passes.
