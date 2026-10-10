# Repo-local wrappers (`.tools/`)

Lets a developer start a tool from the project repo instead of from this checkout.

- `internal/wrappers.Install(repo, toolRoot, project)` writes `.tools/{work,auto_daemon,prepare_for_push}` (bash) and `.bat` twins. Each first `cd`s into `<toolRoot>` (the registry lookup needs the checkout as working directory), then runs `<toolRoot>/bin/<tool> project=<name> "$@"`, so no project menu appears.
- `.tools/.gitignore` contains `*`, so the directory ignores itself; the wrappers hold local absolute paths and are never committed. The target repo's own `.gitignore` is not touched.
- Written automatically when a repo is registered through the menu's paste-a-path flow (`menu.Options.ToolRoot`).
- For existing projects: `work_dot_bat install-wrappers [project=<name>]` (all registered projects when no name). Re-running rewrites stale paths.
- Usage from the repo: `.tools/work`, `.tools/auto_daemon`, `.tools/prepare_for_push` (`.bat` on Windows).
