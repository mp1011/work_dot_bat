---
status: Not Started
---

# Daemon parent close-out can never mark the parent Done

## Context
`closeOutParents` (Node `daemon-run.mjs`, Go `internal/daemon/run.go`) assesses a parent once every subtask is Ready for Review, Done or Cancelled, then runs `plan-status set <parent> Done`. `plan-status set … Done` refuses while any subtask is not Done/Cancelled, and the daemon leaves its own subtasks at Ready for Review. So the parent close-out always fails (Node ignored the failure and committed an empty/no-op change; the Go port now logs "could not be marked Done" and stops).

## Proposal
Decide the intended policy: either the daemon sets the parent to Ready for Review (consistent with "only the user confirms Done"), or close-out waits until the human has marked the subtasks Done. Then change `closeOutParents` and its test (`TestParentCloseOutAssessesAndReportsRefusedDone`).
