---
status: Done
subtask_of: plans/october_2026/initial_port/plan.md
---

# Session analysis and reclassify tools in Go

## Goal
`work_dot_bat analyze_session` and `reclassify_sessions` read agent transcripts and print/update session analyses exactly as they do now.

## Context
Depends on subtask 01.
Source: `src/analyze_session.js`, `src/reclassify_sessions.js`, `src/lib/agent-transcripts.mjs`, `session-analysis.mjs`, `session-names.mjs`, `session-reclassify.mjs`, and tests. Transcript JSONL parsing must tolerate malformed lines as it does today.

## Requirements
1. Transcript discovery per agent engine and OS home-directory convention.
2. The analysis report matches the current format.
3. Reclassification rewrites session names/labels identically.

## Implementation Plan
Stream-decode JSONL into typed structs; keep pure analysis separate from file IO.

## Testing Plan
- Given fixture transcript JSONL files (including a malformed line), the report text matches golden files.
- `reclassify_sessions` on a temp state dir changes only the expected records; observed by diffing the state files.

## Verification Plan
`go test ./...`; run `analyze_session` on a real local transcript and compare with the Node tool.
