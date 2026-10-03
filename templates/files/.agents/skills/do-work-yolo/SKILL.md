---
name: do-work-yolo
description: Non-interactive, general purpose problem solver that routes to a more appropriate skill.
---

See `agent_docs\instructions\high_trust\do-work-yolo.md`.

Mirror each phase/step of the routed instructions as a task via TaskCreate up front, and TaskUpdate them in_progress/completed as you go, so an unattended run is followable.

Never run `git commit` or `git push` — hard-denied in `.claude\settings.json`, human-only.
