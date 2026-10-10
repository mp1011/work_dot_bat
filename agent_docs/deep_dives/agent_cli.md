# Agent selection: `agent_cli.json`

The agents the tool can launch (menu, daemon, push flow) are data, not code. Built-ins live in
`agentcmd.DefaultDefinitions()`; the repo's `agent_cli.json` spells them out, and a project adds or
overrides agents with its own `agent_cli.json` at its root (`agentcmd.LoadDefinitions`, applied by
`agentcmd.ProjectSettings`). `TestRepoAgentCLIFileMatchesBuiltIns` keeps the two in step.

**Every agent is self-contained.** The model and other flags are written into its `args`; there is no
separate model/effort setting. Claude Sonnet and Claude Opus are two agents (`c`, `o`) that share a `cmd`,
not one agent with a parameter. (`agent_settings.json`, `WORK_DOT_BAT_AGENT_SETTINGS` and the config
`agents` block no longer affect agent launching.)

```json
{ "agents": [
  { "id": "claude-opus", "args": ["--model", "opus", "--permission-mode", "auto"] },
  { "id": "gem", "key": "g", "label": "Gem CLI", "cmd": "gem",
    "args": ["-m", "x"], "interactiveArgs": ["{prompt}"], "printArgs": ["-p", "{prompt}"],
    "session": { "reader": "claude", "dir": "~/.gem/sessions" } }
] }
```

- **Invoke:** command line = `args` + (`sessionIdArgs` if a session id) + (`logFileArgs` if a log file) +
  `interactiveArgs` or `printArgs`. Placeholders: `{prompt} {repoRoot} {sessionId} {logFile}`.
- **Override vs add:** an entry with an existing id replaces only the fields it sets; a new id needs `cmd`,
  `key`, and `session.reader`.
- **Read sessions:** `session.reader` picks a transcript format — `claude` (`<dir>/**/<sessionId>.jsonl`) or `agy`
  (brain transcript found via the run's log file); `session.dir` overrides the root; `session.logFile: true`
  records a per-run log file in the ledger and passes it via `logFileArgs`. Ledger rows store `agent` (the id)
  and `reader`, so later reclassification needs no definitions file.
- Agent names (id, key, alias, menu position) resolve through `Settings.Agent`; unknown names fall back to the
  first `claude`-reader agent.
- A missing or invalid file yields the built-ins. New transcript formats need a new reader in `agentsession`.
