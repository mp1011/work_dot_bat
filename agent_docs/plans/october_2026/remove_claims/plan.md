---
status: Done
---

# Remove the plan-claim registry entirely

## Goal
No part of work_dot_bat records, checks, or displays plan/branch claims. `plan-claim.sh`, the `plan-claim` command and the `planclaims` package no longer exist.

## Context
- Follows `plans/october_2026/remove_concurrency_check.md`, which removed the agent-facing check and the launcher gating.
- Decision (user): even the daemon must not use claims. The daemon takes the next available task, and running two daemons on one repository is not supported — that is the human's responsibility, as is running two agents on one plan.
- Remaining users of claims: `src/internal/daemon` (acquire in `run.go`, skip in `plans.go`/`loop.go`, display in `cli.go`), `src/internal/work` (`WithClaim`, `ClaimEnv`, `claims()`, `menuopts.go`), `src/internal/batch` (`WithClaimRelease`), `src/internal/sessiontools`, `src/internal/checks/runchecks.go`, `src/internal/planstatus` (`tasks.go`), `src/cmd/work_dot_bat/main.go`, state in `.work_dot_bat/claims` (`repo`, `bootstrap`, `statemigrate`), the vendored script and `src/vendor/manifest.json`, plus docs/templates.

## Requirements
1. Daemon, launcher, status tools and checks run with no claim reads or writes.
2. `plan-claim` is gone from the CLI, vendor scripts, manifest, `.tools`, and docs.
3. Existing `.work_dot_bat/claims` directories are no longer created or required; stale ones are harmless.

## Implementation Plan
Subtasks, in order: 01 daemon, 02 launcher/status tools, 03 delete the package, script and docs.

## Testing Plan
See subtasks.

## Verification Plan
- `./.tools/run-checks.sh` green.
- `git grep -i "plan-claim\|planclaims"` returns nothing outside `agent_docs/plans/`.
