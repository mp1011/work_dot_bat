---
status: Done
---

# GOAL
Create a single command to quickly give the status of all projects.


# Desired Output
- identify running agent sessions by looking at process list
- identify items in blocked status
- identify items in stalled status (in progress with no running process)
- identify items in ready for review status

## Format
```
<Project : Worktree>
    [STATUS] [Relative Plan Path] - [Agent]
```
- omit "Worktree" if work is done on main branch
- omit " - [Agent]" if it is not known

STATUS Can be:
- Active (there's a live process running)
- Stalled (status is In Progress but no process is running)
- Blocked
- Ready for Review

Other statuses are not given.

## Context
- `src/internal/monitor/proc.go`: OS process inspection for Windows (PowerShell/CIM) and Unix/macOS (`ps -eo pid,args`).
- `src/internal/monitor/monitor.go`: Aggregates project worktree statuses, queries process list, matches active sessions, and formats output.
- `src/internal/monitor/monitor_test.go`: Unit tests for process detection, worktree grouping, agent attribution, and formatting.
- `src/cmd/work_dot_bat/main.go`: Subcommand dispatch for `monitor` / `universal_monitor`.
- `src/cmd/work_dot_bat/main_test.go`: Tests for `work_dot_bat monitor` command.
- `.tools/monitor`, `.tools/monitor.bat`: Command wrapper scripts.
- `agent_docs/deep_dives/universal_monitor.md`: Architecture and usage guide.

## Verification
- Unit tests in `src/internal/monitor` testing active process detection, worktree grouping, agent matching, and format rules.
- Command dispatch tests in `src/cmd/work_dot_bat`.
- Full repo regression test suite passing via `go test ./...` and `go vet ./...`.
- Live test execution of `monitor` successfully detecting running processes (`agy`, `claude`, `codex`) across all registered projects and worktrees.