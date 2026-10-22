---
status: Done
---

# GOAL
Add commands for additional agents. 

Remove the "letter" selection as the list of agents is getting to the point where a single letter isn't enough anymore. Change to numeric.

1. `agy` Gemini 4 Argon
2. `agy` Claude Sonnet (distinct from the existing option which is Sonnet using Claude)
3. `agy` Claude Opus (distinct from the existing option which is Opus using Claude)

## Research
- The built-in agents and definitions are implemented in `src/internal/agentcmd/definition.go` and `agent_cli.json`.
- `ChooseAgent` in `src/internal/agentcmd/agentcmd.go` displays the menu of available agents using `d.Key` and accepts user input matching `Key`, position (`i+1`), `ID`, or `Aliases`.
- Currently keys are letters (`a`, `c`, `o`).
- The `agy` CLI supports models:
  - `gemini-3.8-flash` (with `--effort medium`)
  - `gemini-4-argon` (with `--effort medium`)
  - `claude-sonnet-5-5` (with `--effort medium`)
  - `claude-opus-5-5` (with `--effort medium`)
- `TestRepoAgentCLIFileMatchesBuiltIns` in `agentcmd_test.go` enforces that `DefaultDefinitions()` in `definition.go` and `agent_cli.json` match identically.
- Switching to numeric keys (`"1"`, `"2"`, `"3"`, `"4"`, `"5"`, `"6"`) allows distinguishing all 6 agents cleanly.

## Context
- `src/internal/agentcmd/definition.go`: `DefaultDefinitions()`
- `src/internal/agentcmd/agentcmd.go`: `ChooseAgent()`
- `agent_cli.json`: repo root agent definitions
- `src/internal/agentcmd/agentcmd_test.go`: unit tests for agent selection and command building
- `src/internal/work/app_test.go`: launcher fixture tests using agent selection
- `src/internal/work/push_test.go`: push workflow tests using agent selection
- `agent_docs/deep_dives/agent_cli.md`: documentation on agent configuration

## Implementation Plan
1. Update `DefaultDefinitions()` in `src/internal/agentcmd/definition.go` to include all 6 agents with numeric keys 1-6.
2. Update `agent_cli.json` to match `DefaultDefinitions()`.
3. Update tests in `src/internal/agentcmd/agentcmd_test.go` to reflect numeric keys and test all agents.
4. Update tests in `src/internal/work/app_test.go` and `src/internal/work/push_test.go` to use numeric keys.
5. Update `agent_docs/deep_dives/agent_cli.md` documentation.
6. Verify all tests pass (`go test ./...`) and gate checks pass.

## Testing Plan
- `go test ./src/internal/agentcmd`: unit tests verify `BuildCommand`, `ChooseAgent`, `LoadDefinitions`, `TestRepoAgentCLIFileMatchesBuiltIns`, and model arguments.
- `go test ./src/internal/work`: launcher and push workflow integration tests pass with numeric keys.
- `go test ./...`: all unit tests across the repository pass.
