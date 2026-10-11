---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Mechanical checks and the pre-push workflow in Go

## Goal
`work_dot_bat run-checks` and the individual checks (audit, duplication, structural duplication, monolith, skills, source-assertions, project-structure update) run natively, and the pre-push workflow runs checks, agent checks, then pushes (human-invoked only), with the same reports and exit codes.

## Context
Depends on subtasks 01–02.
Source: `vendor/scripts/work_dot_bat/typescript/check-*.mjs`, `duplication-common.mjs`, `report-structural-duplication.mjs`, `run-checks.mjs`, `update-project-structure.mjs`, the pre-push entry point in `src/`, baselines in `agent_docs/*.json`. The checks analyze target repos' TypeScript/TSX: **decide and record in this plan** whether Go tokenizes TS itself or shells out to Node/TypeScript for AST-dependent checks. Baseline JSON formats must stay compatible.

## Requirements
1. Each check reports violations in the current format and respects existing baselines.
2. `run-checks` orchestrates them with the same ordering and summary.
3. The pre-push workflow keeps its guard rails (never invoked by agents).
4. Monolith/duplication results on this repo match the Node implementation within the baselines.

## Implementation Plan
Port check by check, using Node output on this repo as an oracle.

## Testing Plan
- Fixture repos with a known oversized file / duplicated block / source-text assertion each yield the exact expected violation lines and a non-zero exit; a clean fixture exits 0.
- A baseline listing a violation suppresses it; a new one still fails.
- `run-checks` on fixtures prints the summary in golden form.

## Verification Plan
`go test ./...`; `work_dot_bat run-checks` in this repo compared with `scripts/work_dot_bat/run-checks.sh`.

## Decision (recorded while implementing)
**TypeScript-AST checks stay on Node; everything else is native Go.**
- Native: plan drift (the existing `planstatus` check), monolith, duplication, skills, audit, the project-structure doc regenerator, `run-checks`, and the pre-push workflow.
- Delegated: `structural-duplication` and `source-assertions` need a real TypeScript scanner/parser (`ts.createScanner`, `ts.createSourceFile`). A faithful Go tokenizer/parser would be a large, drift-prone re-implementation, and these checks only ever run against TypeScript projects that already have Node and the `typescript` package (bootstrap reports it as a prerequisite). `run-checks` therefore runs the target repo's synced `scripts/work_dot_bat/typescript/check-*.mjs` under `node`, with the same arguments, and says "run `work_dot_bat sync`" when the script is absent.
- Consequence for cutover (09): the Node scripts for those two checks must stay in the vendored set that `sync` installs, even if the other vendored scripts become shims.

## Outcome notes
- Oracle runs on this repo: `check-skills`, `check-monolith`, `update-project-structure --check` and `run-checks --skip-tests` print byte-identical output to the Node scripts. `check-duplication` could not be compared because this repo has no commits yet (both fail with git's `Not a valid object name HEAD`).
- Commands are binary subcommands (`work_dot_bat run-checks | check-monolith | check-duplication | check-skills | check-audit | update-project-structure`) acting on the repo containing the current directory; baseline JSON formats are unchanged.
