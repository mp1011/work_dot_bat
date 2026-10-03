---
status: Done
subtask_of: plans/initial_port/plan.md
---

# Plan status, claims, and list-tasks in Go

## Goal
`work_dot_bat plan-status ...`, `plan-claim ...` and `list-tasks` do what `scripts/work_dot_bat/plan-status.sh`, `plan-claim.sh` and `list-tasks.sh` do today (add, add-subtask, set, list, list-tasks, check, acquire, release, concurrent-check, prune), with identical output and exit codes.

## Context
Depends on subtask 01.
Source: `vendor/scripts/work_dot_bat/lib/plan-status.mjs`, `plan-claims.mjs`, `plan-claim-cli.mjs`, `worktree-status.mjs`, `src/lib/claim-prune.mjs`, the `*.sh` wrappers, and their `*.test.ts`.
**Open decision for the user:** the vendored `.sh` scripts are copied into target repos and call Node. Proposed: they become thin shims that exec the `work_dot_bat` binary (found via PATH or an env var), versioned through the vendor manifest. Confirm before subtasks 07 and 09.

## Requirements
1. Frontmatter status read/write with the same allowed statuses and the `--human` guard for Cancelled.
2. `add`, `add-subtask -parent`, `regenerate`, `backfill-subtasks`, `ensure-frontmatter`, `set-verification`, `list-changed-done`.
3. Claim files: acquire/check/release/list/steal, TTL staleness (4h `AGENT_TTL_MS`), live-launcher-pid detection, concurrent-check verdict text identical to today.
4. Claim and status files written by Node are readable by Go and vice versa.

## Implementation Plan
Port the data formats first, then the CLI. Reuse fixtures from the `.test.ts` files.

## Testing Plan
- `set` on a temp plan changes only the `status:` line; `Done` by an agent is refused. Observed by reading the file and the exit code.
- `acquire` twice on one plan: the second exits 1; after the TTL (injected clock) it succeeds.
- `concurrent-check` with a live claim prints `BLOCKED  <plan>` plus a reason line and exits 1; with none, exits 0.
- `list-tasks` over a temp tree of plans prints indexed open tasks matching the Node output (golden file).

## Verification Plan
`go test ./...`; run `work_dot_bat list-tasks` in this repo and diff against `scripts/work_dot_bat/list-tasks.sh` output.

**Resolved (2026-10-03, user):** the vendored scripts are thin shims that exec the `work_dot_bat` binary (PATH or `WORK_DOT_BAT_BIN`); implemented in plan 09 (manifest 2.0.0).
