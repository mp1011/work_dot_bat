# SMALL ENHANCEMENT TDD (low-trust)

Follow the 10 phases in `agent_docs\instructions\high_trust\small-enhancement-tdd.md`, with these trust changes:

- Ask the user for permission before moving to the next phase.
- Phase 2 ("Ask questions"): ask the user clarifying questions and for any context you need; record questions and answers in the plan file; offer the `/grill-me` skill.
- Phase 5: escalate to the user (not the non-interactive branch) if you cannot name an observable.
- Phase 6: the user reviews your code and gives feedback.
- Phase 7: explain to the user how to manually test; they give feedback on whether it works.
- Phase 9: also ask the user whether any documentation needs updating or creating.
