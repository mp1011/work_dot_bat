# Assess Done

Independently judge whether a plan's goals and requirements have been fully implemented.

This is a **strictly read-only** assessment. Do not edit any files, do not run git commit or push, and do not change plan status.

## Assessment Protocol

1. **Read the plan file**:
   Read the target plan (e.g. `agent_docs/plans/...`). Identify:
   - The GOAL
   - Technical Requirements / Deliverables
   - Testing Plan

2. **Inspect the actual changes**:
   Examine the branch diff against `main` and inspect the target files on disk:
   - Run `git diff main...HEAD` (or `git log main..HEAD --oneline`, `git status --short`) to see what changes exist on the branch compared to `main`.
   - Inspect the modified or created files to confirm the code exists as specified.

3. **Evaluate deliverables vs requirements**:
   - Compare each requirement and testing criterion against the code and test implementations.
   - **Critical rule**: A source-text assertion (such as searching for a comment, symbol name, or string), a plan's frontmatter status change, or pre-existing green tests is **not** evidence of completion. Judge the deliverable: is the requested functionality actually implemented, hooked up, and verified with genuine automated tests?

4. **Report outcome**:
   - If **all** requirements and test criteria are genuinely satisfied, finish with exactly one final line:
     `ASSESSMENT: complete`
   - If **any** requirement or test criterion is missing, incomplete, or only stubbed out:
     List the concrete gaps as a bullet list (format: `- <requirement>: <what is missing>`):
     - ...
     And finish with exactly one final line:
     `ASSESSMENT: incomplete`
