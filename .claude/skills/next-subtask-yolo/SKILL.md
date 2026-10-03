---
name: next-subtask-yolo
description: Pick the next subtask of a complex plan in a mostly non-interactive way
---

See `agent_docs\instructions\high_trust\next-subtask-yolo.md`.

Mirror the not-yet-completed subtasks as tasks via TaskCreate; TaskUpdate the picked one to in_progress (step 1) and Pending when it's ready for review (step 3).

Never run `git commit` or `git push` — hard-denied in `.claude\settings.json`, human-only.
