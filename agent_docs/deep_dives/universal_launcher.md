# Universal Launcher (`.tools/run`)

The universal launcher lets a user select which mode/command to run for a registered project.

- **Entry point**: `.tools/run` (POSIX) and `.tools/run.bat` (Windows), which forward to `work --run run` (`src/cmd/work_dot_bat/run.go`). `"run"` is also included in `internal/wrappers.Tools` so registered repositories receive `.tools/run` wrappers.
- **Storage**: Launch commands are stored centrally in `projects.json` under each project's `launchCommands` array (`[]projects.LaunchCommand`, each `{ "name": "<description>", "command": { "cmd": "...", "args": [...] } }`). Validation in `projects.ValidateRegistry` shares `validateNamedCommands` with `extraChecks`.
- **Interactive Menu (`internal/launcher`)**:
  - Displays a numbered list of configured launch commands with their names and formatted command lines.
  - Selecting a number runs the command interactively (stdin/stdout/stderr attached) in `project.Path`.
  - Selecting `a` prompts for `Command: ` first, then `Description: `, appends the entry to the project in `projects.json`, and returns to the menu.
  - Selecting `q` (or EOF) exits cleanly.
