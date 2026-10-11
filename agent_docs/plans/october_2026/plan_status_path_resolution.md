---
status: Done
---

# Investigate: how `plan-status.sh` resolves plan paths

## Goal
Establish, from the source and by experiment, exactly which path forms `plan-status.sh` (`work_dot_bat plan-status`) accepts, and decide whether the current behavior needs a fix or only documentation.

## Context
- Observed: `plan-status.sh set agent_docs/plans/october_2026/immediate_inbox/plan.md "Done"` failed with `No entry found for "agent_docs/plans/..."`, while `plan-status.sh set plans/october_2026/immediate_inbox/plan.md "Done"` succeeded. Nothing was modified by the failed call.
- Working hypothesis (verified): paths are keyed relative to `agent_docs/`. `src/internal/planstatus/frontmatter.go:3` and `scan.go:42` mention "agent_docs-relative" paths, and `subtask_of` frontmatter uses the same form.
- The script header (`.tools/plan-status.sh`) documents `<path>` without saying what it is relative to. `CLAUDE.md` and `agent_docs/00_start_here.md` direct agents to use this script, so a wrong guess costs a failed call at best.

### Root Cause Analysis in Code
The inconsistency across subcommands comes from asymmetric normalization:
1. **`add`** calls `NormalizePlanPath(entryPath)`. Because `NormalizePlanPath` strips backslashes and trims an `"agent_docs/"` prefix, `add` accepts both `plans/...` and `agent_docs/plans/...`. However, it does not normalize absolute paths or `./agent_docs/...` (because `TrimPrefix` requires an exact match).
2. **`add-subtask`** calls `NormalizePlanPath` on both `entryPath` and `parentPath` in `cli.go`, and again inside `AddSubtask` in `commands.go`. Thus both the subtask and parent paths accept `plans/...` and `agent_docs/plans/...`.
3. **`set`** does **NOT** call `NormalizePlanPath` on `entryPath` before looking up the file:
   - It executes `readFile(root, entryPath)` which calls `docsPath(root, rel) -> filepath.Join(root, "agent_docs", filepath.FromSlash(rel))`.
   - If `entryPath` is `"agent_docs/plans/..."`, it attempts to read `<repo_root>/agent_docs/agent_docs/plans/...`, which fails with `No entry found for "<entryPath>"`.
   - If `entryPath` is `"plans/..."`, it accesses `<repo_root>/agent_docs/plans/...` and succeeds.
   - For `SetBody` and `ReadEntryStatus`, the same unnormalized `entryPath` is passed to `readFile`.
4. **`set-verification`** also passes `entryPath` directly to `readFile` without calling `NormalizePlanPath`.
5. **Subdirectory and Worktree Invocation**:
   - `work_dot_bat` locates the repository root via `git rev-parse --show-toplevel`.
   - Therefore, running `plan-status.sh` from any subdirectory or git worktree always resolves `c.Root` to the worktree or repo root containing that directory. Relative path arguments are resolved against `filepath.Join(c.Root, "agent_docs", ...)` regardless of the shell's current working directory.

### Findings Matrix: Subcommands vs Path Forms

| Path Form | Example Input | `add` | `add-subtask` | `set` | `set-verification` | Silent Wrong Modification? |
|---|---|:---:|:---:|:---:|:---:|:---:|
| **`agent_docs/`-relative** | `plans/foo/bar.md` | SUT | SUT | SUT | SUT | No |
| **Repo-root-relative** | `agent_docs/plans/foo/bar.md` | SUT | SUT | Fail (`No entry found`) | Fail (`No entry found`) | No |
| **Absolute path** | `/repo/agent_docs/plans/foo/bar.md` | Fail (`No such file`) | Fail (`No such file`) | Fail (`No entry found`) | Fail (`No entry found`) | No |
| **`./` repo-root-relative** | `./agent_docs/plans/foo/bar.md` | Fail (`No such file`) | Fail (`No such file`) | Fail (`No entry found`) | Fail (`No entry found`) | No |
| **`./` docs-relative** | `./plans/foo/bar.md` | SUT | SUT | SUT | SUT | No |
| **Backslash repo-relative** | `agent_docs\plans\foo\bar.md` | SUT | SUT | Fail (`No entry found`) | Fail (`No entry found`) | No |
| **Backslash docs-relative** | `plans\foo\bar.md` | SUT | SUT | SUT | SUT | No |
| **From subdirectory** | Ran from `<root>/src` with `plans/...` | SUT | SUT | SUT | SUT | No |
| **From worktree** | Ran in linked worktree with `plans/...` | SUT | SUT | SUT | SUT | No |

*(Note: "SUT" = Succeeded Upon Test. In no case did any path form silently modify the wrong file; failures return exit code 1 with an error message).*

### Recommendation
**Recommend Option (a): Accept repo-root-relative, absolute, and `./`-prefixed paths by normalizing them to the canonical `agent_docs/`-relative key.**

**Rationale:**
- AI agents and developers frequently pass full repo-relative paths (`agent_docs/plans/...`), paths auto-completed by shells/editors (`./agent_docs/...`), or absolute paths.
- `plan-claim.sh` already resolves paths seamlessly via `planstatus.NormalizePlanPath(rawPlan)`.
- `add` and `add-subtask` already normalize paths, creating a confusing discrepancy where `add agent_docs/plans/...` succeeds, but `set agent_docs/plans/...` fails immediately afterward.
- Enhancing `NormalizePlanPath` (or adding path resolution against `root`) to trim `c.Root`, `./`, and `agent_docs/` cleanly unifies all commands.

**Files to touch in follow-up fix:**
- `src/internal/planstatus/frontmatter.go`: Enhance `NormalizePlanPath` (or add a helper aware of repo root / filepath cleaning) to handle `.` prefixes, repo root prefixes, and `agent_docs/`.
- `src/internal/planstatus/commands.go`: Call path normalization at the entry of `Set`, `SetVerification`, `SetBody`, `ReadEntryStatus`, and `ReadBody`.
- `src/internal/planstatus/cli.go`: Ensure printed output uses the canonical normalized path consistently.
- `src/internal/planstatus/planstatus_test.go`: Add table-driven tests verifying all path forms across `add`, `add-subtask`, `set`, and `set-verification`.

## Requirements
1. Read the path-handling code for every subcommand that takes a path and document the exact resolution rule, including how the lookup key is built and where "No entry found" is raised.
2. Test each path form against a scratch plan file: repo-root-relative, `agent_docs/`-relative, absolute, `./`-prefixed, backslash separators, and invocation from a subdirectory and from a worktree.
3. Report which forms succeed, which fail, and whether any form silently modifies the wrong file.
4. Recommend one of: (a) accept repo-root-relative and absolute paths by normalizing them to the `agent_docs/`-relative key; (b) keep the behavior and improve the error message to name the expected form; (c) documentation only.

## Implementation Plan
- Trace `plan-status` argument handling in `src/internal/planstatus/commands.go` and the entry lookup behind "No entry found".
- Build a table of path forms vs. subcommands from the experiments.
- Write findings into this file's Context section; do not change code in this plan.
- If a fix is recommended, create a follow-up plan rather than implementing here.

## Testing Plan
- No code changes, so no new tests. If a follow-up fix is planned, it must include a table-driven test over the path forms found here.

## Verification Plan
- The findings table covers every path-taking subcommand and every form listed in Requirement 2.
- The recommendation is justified by the findings and names the exact files the fix would touch.
