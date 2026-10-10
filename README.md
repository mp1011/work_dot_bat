# work_dot_bat

Command-line tooling for agent-driven development, as one self-contained Go binary for Windows, macOS and Linux (no Node required to run it):

- `work` — interactive task launcher: pick a plan, launch the configured agent on it, merge a plan's worktree branch back, run the checks, commit and push.
- `auto_daemon` — unattended plan daemon: works every `auto: true` plan in its own git worktree, one agent run per plan, and records the outcome. `auto_status` reports what it would pick up and why the rest are held back.
- `prepare_for_push` — the mechanical checks, then the agent's checks, then a human decision, then `git push`. Human-invoked only.
- `create_worktree` — worktree provisioning for a plan.

It was extracted from music_mountain and ported from Node/TypeScript to Go; the original behavior, output text and file formats are preserved.

## Install

Build for every platform with `scripts/build-release.sh [version]` (binaries land in `dist/work_dot_bat_<os>_<arch>[.exe]`; needs only the Go toolchain), or for this machine with `go build -o work_dot_bat ./cmd/work_dot_bat`. Put the binary on your `PATH`, or set `WORK_DOT_BAT_BIN`. Check a build with `scripts/smoke.sh <binary>`.

To automatically add `bin` to your `PATH`:
- Windows (PowerShell): `.\scripts\add-to-path.ps1`
- Windows (Command Prompt): `.\scripts\add-to-path.bat`
- Linux / macOS / Git Bash: `./scripts/add-to-path.sh`

`bin\*.bat` (Windows) and `bin/*` (POSIX) are thin wrappers that find the binary (`WORK_DOT_BAT_BIN`, then `PATH`, then `dist/`) and pass the tool name through, e.g. `bin\work.bat project=<name>`.

## Commands

    work_dot_bat <tool> [project=<name>] [args]

| Tool | What it does |
|---|---|
| `work`, `auto_daemon`, `prepare_for_push`, `create_worktree` | the interactive launcher, the unattended daemon, the pre-push workflow, worktree provisioning |
| `auto_status`, `analyze_session`, `reclassify_sessions` | read-only reports on plans and agent sessions (`reclassify_sessions --write` relabels ledger rows) |
| `sync [--dry-run] [--yes]` | install/update the vendored scripts in `scripts/work_dot_bat/` (never commits) |
| `bootstrap [--dry-run]` | prepare a repo: agent docs, instructions, skills, safety hook, scripts, baselines, pre-push hook |
| `migrate_state [--apply]` | move legacy `.tools/` and `.claims/` state into `.work_dot_bat/` |
| `rewrite_paths [--apply]` | rewrite old `scripts/<x>.sh` references to `scripts/work_dot_bat/` |

Subcommands that act on the repository containing the current directory (no project needed): `plan-status`, `plan-claim`, `list-tasks`, `worktree-info`, `run-checks [--no-tighten] [--skip-tests] [--cwd <dir>]`, `check-monolith`, `check-duplication`, `check-skills`, `check-audit`, `update-project-structure [--check]`, `safety-hook` (the Claude Code PreToolUse hook), `--version`.

## What target repos receive

`vendor/` holds the canonical copies of the files copied into every target repo (`scripts/work_dot_bat/`), embedded in the binary. They are thin shell wrappers that call `work_dot_bat <subcommand>` (found via `PATH` or `WORK_DOT_BAT_BIN`), so a target repo needs the binary but not this repo. The two checks that analyze TypeScript syntax trees (`check-structural-duplication`, `check-source-assertions`) keep Node implementations there and need Node plus the repo's own `typescript` package.

After editing `vendor/`, record it with `work_dot_bat vendor-bump [--minor|--major]` (run from this checkout); `work_dot_bat vendor-check` fails on an unrecorded change, and a Go test checks the embedded tree against its manifest.

## Projects

Tools run against a registered project: `bin\work.bat project=<name>`, or launch with no argument for a menu (pick a number, or `p` to paste a repository path, which registers it). The registry is `projects.json` next to the binary or in this checkout (gitignored; copy `projects.example.json`; override the location with `WORK_DOT_BAT_REGISTRY`). Each repo's commands live in its own committed `work_dot_bat.config.json`. Unattended callers (the daemon) must pass `project=`.

## Develop

    go build ./... && go vet ./... && go test ./...
    work_dot_bat run-checks

Agent docs are in `agent_docs/`; the code is organised by responsibility under `internal/` (see `agent_docs/deep_dives/task_menu.md` for the package-by-package notes from the port).
