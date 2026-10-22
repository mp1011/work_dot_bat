# Agent selection: `agent_cli.json`

The agents the tool can launch (menu, daemon, push flow) are data, not code. Built-ins live in
`agentcmd.DefaultDefinitions()`; the repo's `agent_cli.json` spells them out, and a project adds or
overrides agents with its own `agent_cli.json` at its root (`agentcmd.LoadDefinitions`, applied by
`agentcmd.ProjectSettings`). `TestRepoAgentCLIFileMatchesBuiltIns` keeps the two in step.

**Every agent is self-contained.** The model and other flags are written into its `args`; there is no
separate model/effort setting. Agents are assigned numeric keys (`1`, `2`, `3`...) rather than single letters
to cleanly differentiate multiple models across both Google Antigravity (`agy`) and Claude Code (`claude`).

The default agents are:
1. `Google Antigravity (Gemini 3.8 Flash)` (`agy`) - key `1`
2. `Google Antigravity (Gemini 4 Argon)` (`agy-gemini-4-argon`) - key `2`
3. `Google Antigravity (Claude Sonnet)` (`agy-claude-sonnet`) - key `3`
4. `Google Antigravity (Claude Opus)` (`agy-claude-opus`) - key `4`
5. `Claude Code (Sonnet)` (`claude-sonnet`) - key `5`
6. `Claude Code (Opus)` (`claude-opus`) - key `6`
7. `OpenAI Codex (GPT-6.1 Sol)` (`codex`, aliases `openai`, `codex-sol`, `gpt-6.1-sol`) - key `7`
8. `OpenAI Codex (GPT-6 Astra)` (`codex-astra`, aliases `astra`, `gpt-6-astra`) - key `8`

Codex passes an explicit `--model` (`gpt-6.1-sol` or `gpt-6-astra`) and uses `--approve-for-me` for automatic approval
review within the workspace sandbox. Install and authenticate the `codex` CLI before selecting it.
Interactive runs use `codex <prompt>`; automation uses `codex exec <prompt>` and saves its final
response with `--output-last-message`. The `codex` reader uses that file for classification and
daemon outcome markers. Interactive runs are classified by exit code; their final response is not
captured. Codex creates its own session IDs, so the ledger's generated ID is not passed to it.

```json
{ "agents": [
  { "id": "claude-opus", "args": ["--model", "opus", "--permission-mode", "auto"] },
  { "id": "gem", "key": "9", "label": "Gem CLI", "cmd": "gem",
    "args": ["-m", "x"], "interactiveArgs": ["{prompt}"], "printArgs": ["-p", "{prompt}"],
    "session": { "reader": "claude", "dir": "~/.gem/sessions" } }
] }
```

- **Invoke:** command line = `args` + (`sessionIdArgs` if a session id) + (`logFileArgs` if a log file) +
  `interactiveArgs` or `printArgs`. Placeholders: `{prompt} {repoRoot} {sessionId} {logFile}`.
  Non-interactive runs also append `printLogFileArgs` when a log file is supplied, allowing
  output flags that must follow a subcommand such as `exec`.
- **Override vs add:** an entry with an existing id replaces only the fields it sets; a new id needs `cmd`,
  `key`, and `session.reader`.
- **Read sessions:** `session.reader` picks a transcript format — `claude` (`<dir>/**/<sessionId>.jsonl`) or `agy`
  (brain transcript found via the run's log file); `session.dir` overrides the root; `session.logFile: true`
  records a per-run log file in the ledger and passes it via `logFileArgs`. Ledger rows store `agent` (the id)
  and `reader`, so later reclassification needs no definitions file.
- Agent names (id, key, alias, menu position) resolve through `Settings.Agent`; unknown names fall back to the
  first `claude`-reader agent.
- A missing or invalid file yields the built-ins. New transcript formats need a new reader in `agentsession`.
