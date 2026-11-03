# Universal Agent Quota & Rate Limits

`work_dot_bat quota` (and `.tools/quota` / `.tools/quota.bat`) queries and aggregates quota usage and rate limit reset windows across all configured AI coding agents: Claude Code, Google AntiGravity, and OpenAI Codex.

## Quota Retrieval Mechanisms

### 1. Claude Code
- **Mechanism**: Invokes `claude --print "/usage"` with stdin closed.
- **Output format**: Emits text reporting:
  - `Current session: <used>% used · resets <Date> (<Timezone>)`
  - `Current week (all models): <used>% used · resets <Date> (<Timezone>)`
- **Parsing**: Extracts the session and weekly used percentages and converts them to remaining percentages (`100 - used`). Parses the localized reset time into a standard `Jan _2, 3:04pm MST` timestamp using IANA timezone data.

### 2. Google AntiGravity (`agy`)
- **Mechanism**: Invokes `agy --print "/usage"` with stdin closed.
- **Output format**: Emits tab-separated tabular data:
  - `<Model Family>\t<Limit Name>\t<Remaining>%\t<RFC3339 Timestamp>`
- **Parsing**: Extracts the model family and window ("Five Hour", "Weekly") into formatted model names (e.g. `Gemini Models (Five Hour)`). Preserves the remaining percentage and formats the RFC3339 timestamp in local time.

### 3. OpenAI Codex
- **Mechanism**: Communicates directly with the `codex app-server` binary over stdio using JSON-RPC 2.0 without requiring any external JavaScript runtime.
- **Protocol**:
  1. Sends `initialize` request:
     `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"work_dot_bat","version":"1.0"}}}`
  2. Sends `account/rateLimits/read` request:
     `{"jsonrpc":"2.0","id":2,"method":"account/rateLimits/read","params":{}}`
  3. Reads responses from stdout until response id `2` is returned.
- **Parsing**: Parses `rateLimits.primary` (300-minute / 5-hour rolling limit) and `rateLimits.secondary` (10080-minute / weekly rolling limit) into remaining percentages (`100 - usedPercent`) and converts unix epoch timestamps (`resetsAt`) to local time strings.

## Output Format

The output follows the universal format:

```
[Agent]
    [Model] [Percent] (Refreshes <Date>)
```

Percentages are dynamically color-coded:
- **Green** (`\x1b[32m`) if remaining quota is above 80%
- **Yellow** (`\x1b[33m`) if remaining quota is above 40% (and $\le 80\%$)
- **Red** (`\x1b[31m`) if remaining quota is $40\%$ or below (e.g. exhausted)

Colors are enabled automatically on interactive terminals (TTY) and can be forced with `--color` or disabled with `--no-color` / `NO_COLOR`.

Example:
```
Claude
    Current session 90% remaining (Refreshes Oct 31, 1:40pm EDT)
    Current week (all models) 87% remaining (Refreshes Nov 3, 7:00am EST)
AntiGravity
    Gemini Models (Weekly) 95% remaining (Refreshes Nov 3, 6:41pm EST)
    Gemini Models (Five Hour) 96% remaining (Refreshes Oct 31, 1:45pm EDT)
    Claude and GPT models (Weekly) 87% remaining (Refreshes Nov 3, 7:52am EST)
    Claude and GPT models (Five Hour) 85% remaining (Refreshes Oct 31, 1:49pm EDT)
    Gemini 4 Argon (Weekly) 0% remaining (Refreshes Nov 5, 8:35pm EST)
Codex
    Primary (5-Hour) 99% remaining (Refreshes Oct 31, 2:01pm EDT)
    Secondary (Weekly) 76% remaining (Refreshes Nov 4, 7:41am EST)
```

## Agent Filtering

The command supports optional filtering by agent name or alias:
- `.tools/quota claude`
- `.tools/quota agy` (or `antigravity`, `gemini`)
- `.tools/quota codex` (or `openai`, `gpt`)
