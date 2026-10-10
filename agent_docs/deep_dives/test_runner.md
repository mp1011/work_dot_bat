# Layered Test Runner Subcommand (`internal/testrunner`)

The `work_dot_bat test [ultra_light|light|heavy|ultra_heavy|all] [project=<name>]` subcommand enables running specific test tiers defined on a project in `projects.json`.

## Core Semantics
- **Hierarchical Execution**: Running a test at any level implies running all lower-level tests first:
  - `ultra_light`: runs `ultra_light` (linters, type checkers).
  - `light`: runs `ultra_light` then `light` (unit tests).
  - `heavy`: runs `ultra_light`, `light`, then `heavy` (integration / e2e tests).
  - `ultra_heavy` (or `all`, or omitted layer): runs `ultra_light`, `light`, `heavy`, then `ultra_heavy` (quality gates).
- **Early Termination on Failure**: If any command within a tier returns a non-zero exit code, execution halts immediately and that exit code is propagated.
- **Unconfigured Tiers**: If a tier is encountered that has no commands configured in `projects.json`, an informative skip notice is printed and execution proceeds to the next tier (or exits 0).
- **Invalid Tiers**: Requesting an unknown layer name prints a usage error listing the allowed tiers (`ultra_light`, `light`, `heavy`, `ultra_heavy`, `all`) and exits with code 2.
- **Real-Time Output and Timing**: Output is streamed directly to stdout and stderr, with per-command elapsed time reported upon completion.

## Workflow Test Integration

The four test tiers are mapped into `work_dot_bat` lifecycle triggers:

1. **Automated Test Gate (`internal/checks`)**:
   - `Env.RunAutomatedTests(ctx, cwd, projectRoot, targetLayers...)` reads test layer commands from `projects.json` for the active project.
   - If `tests` is not defined in `projects.json`, it falls back to legacy `lightTest` and `heavyTest` from `work_dot_bat.config.json`.
   - Mechanical checks and builds precede test commands; any earlier failure skips subsequent test phases.

2. **Pre-Push Quality Gate (`internal/work/push.go`, `prepare_for_push`)**:
   - The push workflow enforces the `ultra_heavy` tier along with all mechanical gates before handing off to the agent or allowing git push to origin.

3. **Loop Mode (`internal/batch/loop.go`) and Auto Daemon (`internal/daemon/run.go`)**:
   - `light` unit tests run after each code-editing iteration.
   - `heavy` tests run before marking a task or plan `Ready for Review`.
   - Failing tests are sent back to the agent with attempt tracking and budget exhaustion protection.

4. **Interactive Launcher Menu (`internal/work/menuopts.go`, `internal/work/app.go`)**:
   - A two-tiered menu under `t` (Run automated tests) allows choosing which test level to run:
     - `u` (or `0`) — Run ultra-light tests (`ultra_light`)
     - `1` (or `l`) — Run light tests (`ultra_light` + `light`)
     - `2` (or `h`) — Run heavy tests (`ultra_light` + `light` + `heavy`)
     - `3` (or `p`) — Run full pre-push test gate (`ultra_heavy`)
     - `b` — Back to previous menu

