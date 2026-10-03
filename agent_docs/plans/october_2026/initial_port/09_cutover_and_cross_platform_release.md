---
status: Done
subtask_of: plans/initial_port/plan.md
verification: manual
---

# Cutover, cross-platform builds, and docs

## Goal
Users install one self-contained `work_dot_bat` binary for Windows, macOS or Linux (no Node required); the `.bat` entry points become thin wrappers and the Node/TypeScript sources are removed.

## Context
Depends on subtasks 01–08 all being Ready for Review.
Touches `bin/*.bat`, `package.json`, `vitest.config.ts`, `src/`, `test/vendor-contract.test.ts` (port to Go), `README.md`, `agent_docs/`, `templates/files/` (docs referring to node/vitest/npm), `.gitignore`. Nothing is committed yet by design; the user commits after the conversion — do not commit.

## Requirements
1. Cross-compile for windows, darwin and linux on amd64 and arm64 (Makefile or script; version stamped via ldflags).
2. `bin/*.bat` remain as wrappers calling the binary; add POSIX equivalents.
3. Remove Node sources and the vitest config; update `agent_docs/00_start_here.md` "How to run things" to Go commands.
4. README and templates updated; `bootstrap` output no longer references Node.
5. `run-checks` passes on the repo.

## Implementation Plan
Do this last. Verify parity by running both implementations on a scratch project before deleting Node.

## Testing Plan
- Each cross-built binary reports `--version`; the host-OS one runs a smoke flow (`list-tasks`, `sync --dry-run`, `run-checks`) in a scratch repo, observed by exit codes and output.
- The `.bat` wrapper launches the project menu (manual verification, steps written in the plan).

## Verification Plan
`go build ./...`, `go vet ./...`, `go test ./...`, `work_dot_bat run-checks`; the cross-compile script exits 0 for all targets; a directory listing shows no `.mjs`/`.ts` files remain.

## Decisions (confirmed by the user)
- **Node sources: deleted outright** (no copy kept; the repo had no commits). Removed: `src/`, `test/`, `node_modules/`, `vitest.config.ts`, `package.json`, `package-lock.json`, `scripts/*.mjs` (their jobs are now `work_dot_bat vendor-bump|vendor-check`, the Go vendor-contract test, and the embedded assets).
- **Target repos get thin shims**: `vendor/scripts/work_dot_bat/*.sh` exec `work_dot_bat <subcommand>` (PATH or `WORK_DOT_BAT_BIN`); the manifest was bumped to 2.0.0 for those. The two TypeScript-AST checks keep their Node scripts (see plan 08).

## Outcome notes
- Added: `--version` (ldflags `-X main.Version`), `scripts/build-release.sh` (windows/darwin/linux × amd64/arm64, all six build), `scripts/smoke.sh` (passes for the host binary: version, bootstrap, sync --dry-run, plan-status, list-tasks, run-checks), POSIX launchers `bin/work_dot_bat` + per-tool wrappers, and `bin/work_dot_bat.bat` + the per-tool `.bat` wrappers now calling the binary.
- **Not changed**: this repo's own `.claude/settings.json` still has the inline Node safety hook (it needs no repo sources). The Go `work_dot_bat safety-hook` replaces it once the binary is on PATH; a failing hook command is non-blocking, so switch only when the binary is reachable.
- The vendored Node check scripts no longer have a test suite (vitest is gone); their TypeScript-AST logic was not ported to Go.

## Manual Verification
1. Build: `scripts/build-release.sh` and confirm six binaries in `dist/`.
2. From a Windows terminal, run `bin\work_dot_bat.bat --version`, then the `work` wrapper in `bin\` with no project: the project menu appears and `q` exits 0.
3. Run the `work` wrapper with `project=<a registered project>`, pick a task and `s` to return; confirm `work_dot_bat plan-claim list` is empty afterwards.
4. In a scratch repo, `work_dot_bat bootstrap` then `scripts/work_dot_bat/plan-status.sh list-tasks` works with the binary on PATH, and fails with a clear message when `WORK_DOT_BAT_BIN` points nowhere.
