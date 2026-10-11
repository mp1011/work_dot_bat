---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Go agent settings, command and prompt building

## Goal
Choosing an agent (claude, gemini, antigravity, ...) yields exactly the same argv, environment and prompt text as today, from the tool-level agent settings overlaid with the project's `agents` overrides, so every launcher mode starts agents identically.

## Context
Part of `04_interactive_work.md`; depends on 01. Source: `src/lib/agent-settings.mjs`, `batch-common.mjs` (`buildAgentInvocation`, `buildDoWorkYoloPrompt`, `buildFailureFollowupPrompt`), `loop-work.mjs` (`buildRetryPrompt`), `work-common.mjs` (`buildGeneralAgentPrompt`, `promptAgentChoice`) and their tests. The launcher-claim note text in prompts must match byte for byte.

## Requirements
1. Settings resolution: defaults, settings file, project overrides, with the same merge rules.
2. `buildAgentCommand` interactive and non-interactive forms, including session id and log file arguments.
3. All prompt builders produce identical text, including the `launcherHoldsClaim` variants.
4. `promptAgentChoice` lists the same options and honors an already selected agent.

## Implementation Plan
New package `internal/agentcmd`: pure functions only, with the single prompt isolated behind the shared `Prompter`.

## Testing Plan
- Golden tests ported from the `agent-settings` and `batch-common` tests: for each agent and mode, the exact argv and env.
- Prompt builders return the exact strings for a sample plan path, with and without `launcherHoldsClaim`.
- A project config with an `agents` override changes only the overridden field.

## Verification Plan
`go test ./...`; a small harness prints every prompt and argv from Go and from Node for a sample plan and the outputs diff clean.
