---
status: Done
subtask_of: plans/initial_port/02_plan_status_and_claims.md
---

# Go plan claims: acquire, check, release, steal, prune

## Goal
`work_dot_bat plan-claim acquire|check|release|list|concurrent-check|prune` behave like `scripts/work_dot_bat/plan-claim.sh`: a second `acquire` on a held plan exits 1, stale claims recover, and `concurrent-check` prints `BLOCKED  <plan>` plus reason lines and exits 1.

## Context
Part of `02_plan_status_and_claims.md`; depends on subtask 01. Source: `vendor/scripts/work_dot_bat/lib/plan-claims.mjs`, `plan-claim-cli.mjs`, `worktree-status.mjs`, `src/lib/claim-prune.mjs` and their tests. Claim file format must stay compatible with Node (4h `AGENT_TTL_MS`, owner, pid, branch). Live-launcher-pid detection needs a cross-platform process-alive check (Windows included).

## Requirements
1. Claim file read/write interoperable with Node in both directions.
2. `acquire [--owner] [--steal]`, `check`, `release`, `list` (with staleness), `prune`.
3. `concurrent-check` verdict text identical to today.
4. TTL and pid-liveness are injectable so tests need no real waiting.

## Implementation Plan
New package `internal/planclaims` (with a small worktree-status helper if `concurrent-check` needs it), wired as the `plan-claim` tool.

## Testing Plan
- Two `acquire` calls on one plan: first prints `ACQUIRED ...`, second exits 1; with the injected clock advanced past 4h the second succeeds.
- `check` prints `FREE <plan>` when unclaimed and exits 1 with holder kind/label/pid/branch/age when held.
- `concurrent-check` with a live claim prints `BLOCKED  <plan>` and a reason line, exit 1; with none, exit 0.
- A claim file produced by Node is honoured by Go `check`, and vice versa.

## Verification Plan
`go test ./internal/planclaims/...`; acquire in Go, then run Node `plan-claim.sh check` on the same plan and see it held.

## Implementation Notes
- Packages: `internal/planclaims` (claims, decision logic, prune, CLI) and `internal/planstatus/worktrees.go` (branch naming, worktree-list parsing, fossil detection, the git-backed worktree collector). Reachable as `work_dot_bat plan-claim <cmd>`; like `plan-status` it runs before project resolution against the cwd's git repo (falls back to the cwd outside git, as Node does).
- `renew` was in the Node CLI but not in this subtask's list; ported too, as `plan-claim.sh` documents it.
- The git-backed worktree collector landed here (not 02c), since `concurrent-check` needs it. `plan-status list`/`check` now pass it too, closing the overlay gap noted in 02a.
- Cross-platform liveness: Windows uses `OpenProcess`/`GetExitCodeProcess`/`GetProcessTimes` (no PowerShell spawn); Unix uses signal 0 and `ps -o lstart=` (only `go vet`ed for linux/darwin from Windows; not run there).
- Verified against Node on this machine: claims written by Go are read by Node (`check`, `list`, `concurrent-check`) and vice versa, including a launcher claim with a live pid (both report BLOCKED) and a dead pid (both prune it). The Go claim file is byte-identical to Node's in shape (a test pins it).
- Known difference: pruning rewrites the ledger with Go's map ordering, so ledger row keys come out alphabetical instead of insertion order. Content is preserved.
