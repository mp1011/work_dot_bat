# Task picker menu (`internal/taskmenu`)

Go port of the Node launcher's "Step 1" menu (`selectTask` in `src/lib/work-common.mjs`). Output is byte-identical to the Node menu for the same plan tree (checked by diffing both with piped input).

- `list.go` — pure: runnable filtering, parent/subtask grouping, priority order, parked suggestions, `FormatTaskLine`. Colour only when `IsTTY`.
- `paths.go` — `ResolvePlanPath` (pasted-path normalisation) and `PlanFolders`.
- `menu.go` — the prompt loop over `menu.IO`. `Select()` returns `Result{Task, Quit}`; Node's `process.exit(0)` on `q` became `Quit: true` so callers decide how to exit.

## Quirks kept on purpose
- Only one level of nesting is listed: a grandchild (e.g. `02a` under `02`, itself a subtask) never appears, as in Node.
- The menu line shows the worktree branch (Node does the same).

## Deliberate differences from Node
- Write-in refuses to overwrite an existing plan file (Node silently clobbered it).
- Paths with a `..` segment are rejected as "not inside agent_docs/" (Node let them through).
- End of input returns "no selection" instead of hanging.

`create_worktree`'s plan mode uses this menu (`worktree.App.pickPlan`).

# Immediate plan (`internal/immediate`)

`agent_docs/plans/immediate.md` is a fixed, one-slot inbox for failed automations.

- **Recording** — `checks.Env.RunAutomatedTests` (the one entry the push gate, loop mode, the daemon and the launcher's test options share) captures each failing check's output and calls `immediate.Record`; a pass calls `immediate.Clear`. Each failure is a `## AUTOMATION: <check name>` section (command + output tail, capped at `MaxOutput`); a repeat failure replaces its own section, others are appended. The file is created via `planstatus.Add`; a failure after `Done` starts a fresh body and sets `Not Started` (`In Progress` is kept).
- **Clearing** — a passing check removes its section; when none remain the Work script sets `Done` through `planstatus.Set` and erases the body (frontmatter kept). A pass of an unrecorded check changes nothing. A recorded non-test check that is no longer in the run's check list (e.g. disabled in the project config) counts as passed, since it can no longer fail; recorded `[tier] ...` test entries are cleared only by their own pass. The agent never sets `Done` (`agentcmd.ImmediatePlanNote` tells it so).
- **Picking** — `work.App.Run` offers a pending (not `Done`) immediate plan once per run, before the menu, with the notice `Immediate plan pending: picking agent_docs/plans/immediate.md`; backing out returns to the menu.
- **Only the main checkout** — runs in a worktree (the daemon's) never record or clear: that file would pollute the branch's diff, and the daemon retries its own failures. A write failure prints a warning and never changes the gate's result.
- Loop mode runs after the launcher's pick, so it inherits the choice; the daemon does not pick the immediate plan.

# Agent command and prompt building (`internal/agentcmd`)

Pure port of `agent-settings.mjs` plus the prompt builders from `batch-common.mjs`, `loop-work.mjs` and `work-common.mjs`; argv and prompt text diffed clean against Node. Settings layer defaults, `agent_settings.json` (env `WORK_DOT_BAT_AGENT_SETTINGS`, else in the tool root) and the project's `agents` block, merged key by key. `projects.ToolRoot()` is the shared tool-root lookup. `ChooseAgent` returns `quit` instead of exiting the process.

# Interactive launcher (`internal/work`, `internal/agentsession`)

`work_dot_bat work project=<name>` runs `work.Main`: Step 1 menu (`taskmenu`), then the Step 2 action menu (`TaskMenuOptions`), mirroring the Node launcher's `main()`.

- **Ctrl-C** — `ExecAgent` hands the child the real stdin/stdout/stderr handles and ignores SIGINT in the launcher while it runs, so the agent (which owns the terminal) decides what Ctrl-C means . SIGTERM (and tests) cancel the context, which kills the agent.
- **Session ledger** — `agentsession` names sessions, derives the UUIDv5 session id, records start/end in `.work_dot_bat/session_ledger.json` (unknown row fields survive), and classifies the end of a run from the Claude/agy transcript (usage-limit bounces exit 0). Rate-limit parsing uses the embedded tz database (`time/tzdata`) because Windows has none.
- **Breakdown detection** (`breakdown.go`) is ported for the loop/batch/daemon flows (04e, 05); the interactive launcher itself does not call it.
- **Hooks still to port** — `App.MergeBack` (04d), `Loop` (04e), `Push` and `RunTests` (08) print a "not yet ported" notice until wired.

Known sharp edge: on Windows, Go refuses to run `.cmd`/`.bat` shims with arguments it cannot escape safely (the prompt has newlines and quotes). `claude.exe`/`agy.exe` are fine; an npm `.cmd` shim would fail to start (reported as `[ERROR] Failed to execute …`), as Node ≥ 20.12 does without a shell.

# Merge-back (`internal/worktree/merge.go`, `internal/work/mergeback.go`)

`worktree` holds the pure gate (`MergeAvailability`, dirty checkout first) and the git helpers; `work.App.mergeBackWorkflow` is the interactive flow. Tested against real temp git repos with real worktrees: a clean branch ends staged and uncommitted; a conflict is left in progress (`MERGE_HEAD` present) with the paths listed; a locked worktree or an unmerged branch makes cleanup report git's refusal and leave things alone. Cleanup only ever runs `git worktree remove` and `git branch -d`.

Differences from Node: the final "Open …plan_index.html" line is printed only when that file exists; the gate's `MergeMenuLabel` is ported but, as in Node, the action menu always shows plain "Merge back into main" and the reason is shown when the option is chosen.

# Loop and batch logic (`internal/batch`)

Pure, injected-side-effect port of `loop-work.mjs` and the shared parts of `batch-common.mjs`: `RunPlanWithRetries` (also for the daemon, 05), `RunLoopUntilDone`, `NextLoopDecision`, `FormatSummary`, `RateLimitStopMessage`, `PauseForRateLimit` (injected clock and sleeper). Rate-limit text parsing and exit classification live in `agentsession`. `work.App.loopUntilDone` wires it to non-interactive agent runs (`RunAgentNonInteractive`, `--print`, one ledger row per attempt).

Loop mode refuses to start while `App.RunTests` is nil (the check runner is subtask 08): calling work "done" without running the tests would be worse than not running. Pause-and-resume after a bounce is the daemon's job (05); loop mode stops, as in Node.
Path ordering uses byte comparison where Node used `localeCompare`; it only differs for exotic filenames.

When `RunLoopUntilDone` begins, it checks the parent plan's frontmatter for a `plan_order` array. If absent, it invokes the `/plan-order` agent skill once on the parent plan to determine and persist a specific task execution order across direct and nested subtasks. The loop then chooses each incomplete (`IsSubtaskEligible`) subtask in `plan_order` sequence. If `plan_order` is absent or empty, it falls back to default eligible subtasks (`GetEligibleSubtasks`).


# Auto daemon and status (`internal/daemon`)

- `plans.go` — the pure planner: `Eligibility` (auto or auto-ancestor; no own subtasks; Not Started/In Progress; committed; earlier siblings resolved; `depends_on` resolved), `DaemonBranchFor`, `ReadPlanStates` (worktree copy wins when the plan's daemon branch has a live worktree), `FormatAutoStatusReport`.
- `agent.go` — the `AUTO_OUTCOME:` / `ASSESSMENT:` parsing contract (last marker wins) and the daemon prompts, byte-compatible with Node.
- `run.go` — `Runner.RunPlan`: ensure/bootstrap worktree, agent → outcome → tests → non-plan-diff check → `/assess-done`, up to 3 attempts, usage-limit waits (a bounce is not an attempt), parent close-out. Every external effect is a field, so tests inject a fake agent against real git worktrees.
- `proc*.go` — `RunProcess`: stdout/stderr pass-through, stdin closed, heartbeat, timeout, and **process-tree kill** (own process group + SIGTERM→SIGKILL on Unix, `taskkill /T /F` on Windows). Cancelling the context (SIGINT/SIGTERM) kills the agent and its children. Tested by observing a grandchild's absence.
- A stuck agent (timeout) ends the daemon with exit 1, as in Node.
- The daemon refuses to mark a plan done without a test runner (`Config.RunTests`); it is wired with the checks subtask (08).

Differences from Node: the blocked-plan banner uses the plan's `## Blocked` reason (Node always printed the generic text); a refused parent close-out is logged instead of silently committed (see agent_suggestions/daemon_parent_closeout_refused.md).

# Session analysis tools (`internal/sessiontools`, `internal/agentsession`)

`analyze_session` and `reclassify_sessions` operate on the **selected project's** ledger (`<project>/.work_dot_bat/session_ledger.json`); the Node tools resolved it relative to the tool checkout, which only worked when the two were the same repo. The report text is identical to Node's (checked against `formatSessionReport` on the same fixture).

The ledger is now read and written with key order preserved (`agentsession.Row`, `ReadRows`/`WriteRows`): a rewrite only changes the fields it means to, so `reclassify --write` diffs show just the relabelled rows. New keys are appended in Node's order (`status`, `exitCode`, `resetAt`, `endedAt`). The Node regexes that paired quotes with a back-reference are split into quote-style alternatives, since RE2 has none.

# Sync, bootstrap, migrate_state, rewrite_paths, safety hook (`internal/{managed,syncer,bootstrap,statemigrate,pathrewrite,safetyhook,maintcli}`)

- **Assets are embedded** (`assets.go` at the module root: `//go:embed all:vendor` and `all:templates/files`), so the binary carries what `sync` installs and `bootstrap` renders. The vendored *content* is deliberately left as it is (Node scripts); what target repos should receive instead (thin shims that exec this binary, per the open decision in the initial-port plans) is a data change under `vendor/` plus `work_dot_bat vendor-bump`, not a code change.
- `managed` — header render/parse, SHA-256 of the EOL-normalised body, versions; and the manifest (drift, bump). `work_dot_bat vendor-check` / `vendor-bump [--minor|--major]` replace `npm run vendor:check|bump` (they read `./vendor` on disk, run from the tool checkout). `internal/maintcli` has a test that the *embedded* tree matches its manifest, replacing `test/vendor-contract.test.ts`.
- `syncer` — classify (missing/current/outdated/modified/unmanaged/newer), apply, orphans; `--dry-run`, `--yes`, per-file prompts on a terminal; non-zero exit when something was skipped.
- `bootstrap` — templated files (unknown `{{placeholder}}` is an error), baselines for enabled checks, merged settings, `.gitignore`, pre-push hook, then sync. Never overwrites, never commits.
- `safetyhook` — the settings merge now preserves the user's key order and untouched values, and the hook command resolves the binary via `WORK_DOT_BAT_BIN` then `PATH`, failing closed (exit 2) if not found. The hook itself runs `safety-hook` (stdin JSON → exit 2 + message), blocking git commit/push as well as the binary's own push and work subcommands. A legacy inline Node hook counts as installed. This repo's own `.claude/settings.json` uses this fail-closed hook.
- `pathrewrite` — RE2 has no lookarounds, so the "stands alone" boundary check is a small scanner with the same semantics.

# Mechanical checks and the pre-push workflow (`internal/checks`, `internal/work/push.go`, `internal/proc`)

- `checks` ports monolith (ratchet + breakup-plan stubs), duplication (8-line windows, deterministic pair order), skills, audit, the project-structure reconciler (parse-and-preserve, with Node's test cases ported) and `run-checks` (ordering, phase skipping, summary text). Tool-owned checks run in-process; the two TypeScript-AST checks run the target's synced Node scripts (decision recorded in `plans/initial_port/08_checks_and_prepare_for_push.md`).
- `Env.RunAutomatedTests(ctx, cwd, projectRoot, targetLayers...)` is the entry the daemon, loop mode, the launcher's test options and the push workflow share; it checks a worktree in the worktree, reads layer commands from `projects.json` (falling back to `work_dot_bat.config.json`), and returns failing check names.
- `internal/proc` is the shared "run unattended" runner extracted from the daemon: stdin closed, timeout, heartbeat, process-tree kill (own process group + SIGTERM→SIGKILL on Unix, `taskkill /T /F` on Windows).
- `work.App.pushWorkflow` is `doPushWorkflow`: gate → (interactive agent fix → commit if dirty → re-gate → ask) → agent prepare-for-push → ask → `git push`. A failed gate never reaches a push; no gate configured refuses. The standalone `prepare_for_push` tool exits 0 only when it pushed, and the safety hook keeps agents from running it.

# Cutover (plan 09)

The Node/TypeScript implementation is gone (deleted at the user's explicit instruction; the repo had no commits to recover it from). What remains of Node: the four vendored scripts the two TypeScript-AST checks need (`check-structural-duplication`, `check-source-assertions` and their helpers), shipped to target repos and run by `run-checks` under the target's own Node.

- Target repos receive thin shims (`scripts/work_dot_bat/*.sh` → `work_dot_bat <subcommand>`, binary from `PATH` or `WORK_DOT_BAT_BIN`; exit 127 with a message when missing). Manifest bumped to 2.0.0. This repo's own `scripts/work_dot_bat/` was re-synced the same way, so agents running `scripts/work_dot_bat/plan-status.sh` here need the binary reachable.
- `scripts/build-release.sh [version]` cross-compiles six targets (CGO off, `-trimpath`, `-X main.Version`); `scripts/smoke.sh <binary> [version]` runs a scratch-repo flow with a throwaway registry. `bin/` holds POSIX and `.bat` launchers that locate the binary.
- This repo's `.claude/settings.json` uses the fail-closed safety hook that resolves the binary via `WORK_DOT_BAT_BIN` then `PATH`.
