---
status: Done
subtask_of: plans/initial_port/02_plan_status_and_claims.md
---

# Go plan-status: frontmatter, set, add, and list

## Goal
`work_dot_bat plan-status set|add|add-subtask|list|set-verification|ensure-frontmatter|backfill-subtasks|check` behave like `scripts/work_dot_bat/plan-status.sh` today: same output lines, same exit codes, same refusals (an agent may not set `Cancelled` without `--human`; `Done` is refused only while subtasks are open).

## Context
Part of `02_plan_status_and_claims.md`; depends on subtask 01. Source: `vendor/scripts/work_dot_bat/lib/plan-status.mjs` (and `plan-status.test.ts` for fixtures). The `regenerate`/`plan_status.js` shim is only ported if still needed after the cutover decision in 02's open question; flag it rather than guessing. Plan paths are accepted both as `plans/...` and `agent_docs/plans/...`; today the latter errors with `No entry found` unless normalised — keep Node's behaviour and record any difference.

## Requirements
1. Frontmatter parse/write touching only the `status:`/`subtask_of:`/`verification:` lines; allowed-status set and `--human` guard identical to Node.
2. `add`, `add-subtask -parent`, `set`, `set-verification`, `list [status]`, `ensure-frontmatter`, `backfill-subtasks`, `check [--stale-days]`.
3. Files written by Node are readable by Go and vice versa (round-trip a fixture both ways).

## Implementation Plan
New package `internal/planstatus`, wired into `cmd/work_dot_bat` as the `plan-status` tool. Port data handling first, then the CLI. Reuse the `.test.ts` fixtures.

## Testing Plan
- `set` on a temp plan changes only the `status:` line; observed by diffing the file before/after and the exit code.
- `set <plan> Cancelled` without `--human` is refused with Node's message and exit 1; `set <parent> Done` with an open subtask is refused listing it.
- `add-subtask` writes `subtask_of:` and prints `Added subtask <path> (subtask of <parent>) (Not Started)`.
- `check` on a tree with one frontmatter-less plan exits 1 naming that file.

## Verification Plan
`go test ./internal/planstatus/...`; run the Go and Node `list` against this repo and diff the output.

## Implementation Notes
- Package `internal/planstatus`; reachable as `work_dot_bat plan-status <cmd>` (handled before project resolution, so there is no `Project:` banner and it acts on the git repo of the cwd, matching `plan-status.sh`).
- Worktree overlay (`ApplyWorktreeStatuses`, divergence, Done-parent checks) is ported as pure functions; the git-backed collector is injected (`CLI.Collect`, nil = no overlay). Until 02c/03 supply it, `list` and `check` omit worktree data. `list-tasks` and `list-changed-done` print "not yet ported" (02c).
- Byte-for-byte match with Node confirmed: `list` and `check` in this repo diff clean, and a scratch repo round-tripped `add`/`set` between the Go and Node implementations.
- Not ported: nothing from the 02a scope; `regenerate`/`plan_status.js` shim is kept (it only writes when `agent_docs/plan_index.html` exists).
