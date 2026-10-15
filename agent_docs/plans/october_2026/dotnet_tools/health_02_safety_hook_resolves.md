---
status: Done
---

# Make the Claude safety hook actually run

## Goal
The `PreToolUse` safety hook in `.claude/settings.json` runs on every Bash/PowerShell call. If it can't run, the tool call is blocked instead of silently allowed.

## Context
- Recommendation 2 of `agent_docs/project_health/project_health_2026_10_14.md`.
- The hook command is `work_dot_bat safety-hook`. `work_dot_bat` is not on PATH (only `WORK_DOT_BAT_BIN` is set), so the command exits 127. Claude Code treats any exit code other than 2 as a non-blocking error, so the hook does nothing today.
- Git safety still holds through the `permissions.deny` rules for `git commit*` / `git push*`. Whatever else `safety-hook` guards is not enforced.
- The pre-push hook (`run-checks.sh`) does honor `WORK_DOT_BAT_BIN`, so it works.
- `.claude/settings.json` may be written by `work_dot_bat sync` or `bootstrap`; if so, fix the generator, not the file.

## Requirements
1. The hook resolves the binary the same way `run-checks.sh` does (`WORK_DOT_BAT_BIN`, then PATH), or `work_dot_bat` goes on PATH.
2. If the binary can't be found, the hook exits 2 with a message (fail closed).
3. The same holds for the Gemini/agy hook configuration, if one exists.

## Implementation Plan
The user chooses: put the binary on PATH (an environment fix, no code), or change the hook command (a `work_dot_bat` change). Requirement 2 needs the second.

## Testing Plan
Feed the hook a `git push` tool call on stdin and observe exit 2. Temporarily unset `WORK_DOT_BAT_BIN` and confirm a harmless command is blocked with the "binary not found" message.

## Verification Plan
`echo '{"tool_name":"Bash","tool_input":{"command":"git push"}}' | <hook command>` exits 2.
