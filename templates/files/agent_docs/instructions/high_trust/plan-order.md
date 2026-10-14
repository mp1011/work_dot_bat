# Plan Order

Determine a specific execution order for a parent plan's subtasks (including nested subtasks) and write it to an array in the parent plan's frontmatter under `plan_order`.

## Protocol

1. **Target Identification**:
   - The user passes a parent plan path (e.g. `/plan-order agent_docs/plans/foo/plan.md` or `plans/foo/plan.md`).
   - If not provided, ask the user for the parent plan file.

2. **Subtask Discovery**:
   - Find all subtasks associated with the parent plan:
     - Direct subtasks carrying `subtask_of: <parent_path>` in their frontmatter.
     - Nested subtasks: recursively find subtasks of sub-plans (e.g. any subtask whose `subtask_of` points to a child sub-plan of this parent).
     - Inspect the plan's directory and any subdirectories for subtask plan files (`.md`).
   - Read each subtask's `# GOAL`, context, and frontmatter (`priority:`, `depends_on:`, `blocked_by:`).

3. **Dependency and Ordering Analysis**:
   - Trace prerequisite relationships between tasks:
     - Explicit dependencies: `depends_on:` and `blocked_by:`.
     - Structural dependencies: e.g. interfaces and data models before business logic; backend before frontend; core utilities before consumers; leaf tasks before their composite parent subtasks.
     - Priorities: higher priority / foundation tasks earlier.
   - Construct a linear ordering of subtask paths (using `agent_docs/`-relative paths, e.g. `plans/.../task.md`).
   - Every eligible subtask must appear in the list.

4. **Update Parent Plan Frontmatter**:
   - Write the ordered list into the parent plan's YAML frontmatter under `plan_order:` as a YAML array:
     ```yaml
     ---
     status: In Progress
     plan_order:
       - plans/example/01_foundation.md
       - plans/example/nested/01_subtask.md
       - plans/example/nested/plan.md
       - plans/example/02_feature.md
     ---
     ```
   - Preserve all existing frontmatter fields and the body of the parent plan file.
   - Do not commit or push to git.
