---
status: Done
---

# GOAL
Create a command to list the quotas of each agent (Claude, AntiGravity, Codex)

# Detail
Look to see if each agent offers a command line option. Otherwise, it may require running a skill such as /usage.

# Format

```
[Agent]
    [Model] [Percent] (Refreshes <Date>)
```

## Research
- **Claude Code**: Running `claude --print "/usage"` with empty stdin outputs session and weekly usage lines:
  - `Current session: <used>% used · resets <Date>`
  - `Current week (all models): <used>% used · resets <Date>`
- **Google AntiGravity**: Running `agy --print "/usage"` outputs tab-separated rows:
  - `<Model Family>\t<Limit Window>\t<Remaining>%\t<RFC3339 Timestamp>`
- **OpenAI Codex**: `codex app-server` over stdio communicates via JSON-RPC. Sending `initialize` then `account/rateLimits/read` returns `rateLimits.primary` (300 mins / 5-hour window) and `rateLimits.secondary` (10080 mins / weekly window) with `usedPercent` and unix epoch `resetsAt` timestamps.

## Context
- `src/internal/quota/quota.go`: Core quota collection, parsing, normalization, and formatting.
- `src/internal/quota/quota_test.go`: Unit tests for parsers, formatters, and runner mocks.
- `src/cmd/work_dot_bat/main.go`: Subcommand dispatch for `quota` / `quotas`.
- `src/cmd/work_dot_bat/main_test.go`: Tests for `work_dot_bat quota` command.
- `.tools/quota`, `.tools/quota.bat`: Wrapper scripts for developer convenience.

## Implementation Plan
1. Create `src/internal/quota` package with typed data structures (`AgentQuota`, `ModelQuota`) and output formatter matching the required specification.
2. Implement parser and runner for Claude Code (`claude --print "/usage"`).
3. Implement parser and runner for Google Antigravity (`agy --print "/usage"`).
4. Implement parser and runner for OpenAI Codex (`codex app-server` JSON-RPC `account/rateLimits/read`).
5. Support running queries concurrently across agents with graceful error handling when any CLI is absent or errors.
6. Wire `runQuota` into `src/cmd/work_dot_bat/main.go` and add `.tools/quota` / `.tools/quota.bat`.

## Testing Plan
- Unit tests with table-driven tests for parsing real and synthetic Claude, AntiGravity, and Codex outputs.
- Unit tests verifying exact output formatting and indentation:
  `[Agent]`
  `    [Model] [Percent] (Refreshes <Date>)`
- Main command execution tests in `main_test.go`.
- Live smoke test running `.tools/quota.bat`.
