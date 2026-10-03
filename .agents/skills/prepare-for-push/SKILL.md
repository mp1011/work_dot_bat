---
name: prepare-for-push
description: Ensure the project is in a production-ready state before pushing to the main branch
---

See `agent_docs\instructions\low_trust\prepare-for-push.md`.

Mirror each check in that document as a task via TaskCreate so the user can see progress and results.

Never run `git commit` or `git push` — hard-denied in `.claude\settings.json`, human-only. This skill only prepares the project for the user to push.
