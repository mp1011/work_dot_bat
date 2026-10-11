---
status: Done
---

# PROBLEM

The work menu is only capable of displaying one level of hierarchy for subtasks. It can show a subtask of a main task, but not a subtask of a subtask.

# Desired Behavior
The menu should look like this, indenting as much as is needed
  10) [Not Started] plans/parent/plan.md
     ↳ 11) [Not Started] plans/parent/child/plan.md
        ↳ 12) [Not Started] plans/parent/child/grandchild/plan.md
     
# Additional Requirements
- if there is only ONE md file in a folder, and the file is named "plan.md" or "readme.md", then omit that name. The result would look like this:

  10) [Not Started] plans/parent
     ↳ 11) [Not Started] plans/parent/child
        ↳ 12) [Not Started] plans/parent/child/grandchild