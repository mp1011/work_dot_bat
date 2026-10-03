---
status: Done
subtask_of: plans/initial_port/02_plan_status_and_claims.md
---

# Go list-tasks and list-changed-done

## Goal
`work_dot_bat list-tasks [--free]` and `plan-status list-changed-done [ref]` print what `scripts/work_dot_bat/list-tasks.sh` and `plan-status.sh` print today, including claim and worktree annotations and the parked-suggestions footer.

## Context
Part of `02_plan_status_and_claims.md`; depends on 02a and 02b. Source: `list-tasks` and `list-changed-done` sections of `plan-status.mjs`, `worktree-status.mjs`, `list-tasks.sh`.

## Requirements
1. Indexed open-task listing in the same order (priority, then filename), with `CLAIMED by ...` annotations.
2. `--free` filters claimed and worktree-advanced tasks.
3. `list-changed-done` against a git ref (default `origin/main`).

## Implementation Plan
Build on `internal/planstatus` and `internal/planclaims`; shell out to `git` for ref and worktree data.

## Testing Plan
- Temp tree with one Done, one In Progress claimed, one Not Started: output lists the latter two, indexed, the claimed one annotated; matches a golden file captured from Node.
- `--free` omits the claimed task.
- A temp git repo where a plan changed to Done vs `HEAD~1` appears in `list-changed-done HEAD~1`.

## Verification Plan
`go test ./...`; diff `work_dot_bat list-tasks` against `scripts/work_dot_bat/list-tasks.sh` in this repo.

## Implementation Notes
- `internal/planstatus/tasks.go`; reachable as `work_dot_bat plan-status list-tasks|list-changed-done` and the top-level alias `work_dot_bat list-tasks`.
- `planstatus` cannot import `planclaims` (the reverse dependency exists), so claims reach `list-tasks` through an injected `CLI.Claims` hook; `planclaims.Env.ListTaskClaims` supplies it.
- Verified against Node in this repo: `list-tasks` and `list-tasks --free` output is byte-identical, including with a held claim; `list-changed-done` stdout is identical.
- Difference: with an unknown ref, Node leaks git's `fatal: ambiguous argument` message to stderr before falling back; Go stays quiet. Stdout and exit code match.
- Order is path-sorted like Node; priority ordering belongs to the interactive launcher menu (subtask 04).
