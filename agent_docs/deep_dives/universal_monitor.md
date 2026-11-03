# Universal Project Monitor

`work_dot_bat monitor` (and `.tools/monitor` / `.tools/monitor.bat`) provides an aggregated overview of task and session status across all registered projects and active git worktrees.

## Inspection & Status Resolution

The monitor inspects projects registered in `projects.json` (or specified via `project=<name>`):

1. **Process Inspection**:
   - On Windows: Queries `Win32_Process` via PowerShell to retrieve active process IDs and command lines.
   - On Unix / macOS: Runs `ps -eo pid,args` to collect active process IDs and command lines.
   - Matches running processes against plan paths and project paths to detect active AI agent sessions (`agy`, `claude`, `codex`).

2. **Status Mapping**:
   The report only displays items in actionable or reviewable states:
   - **Active**: Plan status is `In Progress` and a live agent process is detected for it in the OS process list.
   - **Stalled**: Plan status is `In Progress` but no active process is running.
   - **Blocked**: Plan status is `Blocked` or `Blocked - User Input Required`.
   - **Ready for Review**: Plan status is `Ready for Review`.
   - Other statuses (`Not Started`, `Done`, `Cancelled`, `Suggestion`) are omitted.

3. **Agent Attribution**:
   - If an agent process is actively running, the agent is extracted from the command line (`agy`, `claude`, `codex`).
   - If the task is not currently running, the agent is retrieved from the project's session ledger (`.work_dot_bat/session_ledger.json`).
   - If the agent is unknown, the agent suffix is omitted.

## Output Format

The output follows the required specification:

```
<Project : Worktree>
    [STATUS] [Relative Plan Path] - [Agent]
```

- When work is on the main branch, `Worktree` and the separator are omitted:
  `<Project>`
- When work is in a worktree branch:
  `<Project : Worktree>`
- If the agent is not known, `" - [Agent]"` is omitted:
  `    [STATUS] [Relative Plan Path]`

### Example

```
work_dot_bat
    [Active] plans/november_2026/universal_monitor/plan.md - agy
    [Ready for Review] plans/october_2026/auto_daemon_error.md - agy-claude-sonnet

Atom : b8a1-dissipative-approach-cushion
    [Blocked] plans/B-particle-rules/B8-falling-rocks/B8a-retune-water-container.md - agy-gemini-4-argon
    [Ready for Review] plans/B-particle-rules/B8-falling-rocks/B8a1-dissipative-approach-cushion.md - agy-gemini-4-argon
    [Stalled] plans/README.md - agy

BackStage
    [Active] plans/november_2026/misc_issues/plan.md - claude
```
