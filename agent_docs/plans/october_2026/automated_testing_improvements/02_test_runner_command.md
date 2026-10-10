---
status: Done
subtask_of: plans/october_2026/automated_testing_improvements/plan.md
---

# Test Runner Subcommand for Layered Test Execution

## Goal
Users and agents can run `work_dot_bat test [layer] [project=<name>]` from the command line to execute a specific test layer (`ultra_light`, `light`, `heavy`, `ultra_heavy`, or `all`), displaying real-time command output, test timing, and returning exit code 0 on pass or non-zero on failure.

## Context
Part of `agent_docs/plans/october_2026/automated_testing_improvements/plan.md`.
Currently, running tests is done via `run-checks` (which runs all checks and test suites in `work_dot_bat.config.json`).
There is no direct command to execute a single test layer (e.g. running only the fast `ultra_light` linters or only the unit tests `light`).
The test runner executes commands within the target project's working directory using the configuration loaded from `projects.json`.
Relevant files: `cmd/work_dot_bat/main.go`, `internal/tools/tools.go`, `internal/testrunner/` (new package) or `internal/checks/`.

## Requirements
1. Add a `test` tool/command to `work_dot_bat` with usage: `work_dot_bat test [ultra_light|light|heavy|ultra_heavy|all] [project=<name>]`.
2. Defaulting: if no layer is specified, default to `all` (running all configured tiers in order: ultra_light, light, heavy, ultra_heavy).
3. Execution: execute the command array defined for that layer in `projects.json` in the resolved project directory, streaming stdout and stderr to the console.
4. Unconfigured layer behavior: if a requested layer is not configured for the project, print an informative notice and exit 0 (or exit non-zero with a clear error if invoked explicitly with an invalid layer name).
5. Pass/fail semantics: propagate the exit code of the underlying command so scripts, git hooks, and agents can reliably detect failures.

## Implementation Plan
Create a test runner package (e.g. `internal/testrunner`) that takes a `Project`, layer name, working directory, and stdout/stderr writers. Register the `test` tool in `internal/tools`. Wire CLI argument parsing and error reporting.

## Testing Plan
- Layer dispatch: Executing `test ultra_light` runs only the `ultra_light` command and does not trigger `heavy` or `ultra_heavy`. Observed via mock runner recording invoked command names and args.
- Failure handling: A failing test command returns its non-zero exit code to the test runner caller, observed via the CLI return code.
- Unconfigured tier: Requesting a tier that has no command configured prints a skip message and exits 0.
- Invalid tier: Requesting an unknown tier name prints a usage error listing the allowed tiers (`ultra_light`, `light`, `heavy`, `ultra_heavy`, `all`) and exits 2.

## Verification Plan
`go test ./internal/testrunner/...` passes; `go run ./cmd/work_dot_bat test light project=work_dot_bat` executes the unit tests and reports success.
