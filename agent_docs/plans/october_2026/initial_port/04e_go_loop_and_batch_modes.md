---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Go loop and batch modes with retries and rate limits

## Goal
The loop mode (run every eligible subtask of a parent until done) and batch mode run agents non-interactively with the same retry, failure follow-up prompt and rate-limit pause/stop behavior, and print the same summary.

## Context
Part of `04_interactive_work.md`; depends on 04b and 04c. Source: `src/lib/loop-work.mjs` (`isSubtaskEligible`, `getEligibleSubtasks`, `runAgentNonInteractively`, `runLoopUntilDone`), `batch-common.mjs` (`MAX_ATTEMPTS`, rate-limit detection and reset parsing with time zones, `classifyAgentExit`, `nextLoopDecision`, `runPlanWithRetries`, `withClaimRelease`, `formatSummary`), `agent-transcripts.mjs`, `session-names.mjs`, and their tests. Reset times like `resets 3pm (America/Los_Angeles)` need IANA time zone handling via Go's `time` package.

## Requirements
1. Eligibility and ordering of subtasks identical.
2. Up to `MAX_ATTEMPTS` attempts, with the failure follow-up prompt built from the check output.
3. Rate-limit text detection, reset-time parsing and pause length match; a stop message when the wait is too long.
4. Claims are released on every exit (`withClaimRelease` semantics).
5. Summary output matches.

## Implementation Plan
New package `internal/batch` with the pure classification and parsing first (golden-tested), then the runner over an injected agent runner.

## Testing Plan
- Table tests ported from `batch-common.test.ts` for rate-limit parsing (including AM/PM, minutes, zones) and `classifyAgentExit`.
- A fake agent runner that fails twice then succeeds yields three attempts, two follow-up prompts, a `done` result and a released claim; one that always fails stops after `MAX_ATTEMPTS`.
- A fake agent output containing the rate-limit message pauses for the parsed duration (injected clock and sleeper) and resumes.
- The loop over a parent with three subtasks runs them in order and skips ineligible ones, observed through the recorded invocations.

## Verification Plan
`go test ./...`; run loop mode against a scratch project with a stub agent script.
