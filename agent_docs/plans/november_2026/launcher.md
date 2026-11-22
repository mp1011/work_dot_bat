---
status: Done
---

# GOAL
Each project has its own command line to run in various modes. Create universal launcher, allowing the user to select which mode to run the application in.

# Detail
- launched by typing .tools\run
- presents the user a numbered menu of launch commands and their names
- names stored centrally in this repository's "projects.json" file
- special menu option to add new entries for the current repository
    - first asks the command
    - then asks the description


# Research
- `.tools/` contains POSIX bash scripts and `.bat` twins (e.g. `.tools/monitor`, `.tools/monitor.bat`, `.tools/work`, `.tools/work.bat`) that invoke the `work_dot_bat` Go binary via `work --run <subcommand> "$@"`.
- `src/internal/wrappers/wrappers.go` installs repo-local `.tools/<tool>` wrappers (`work`, `auto_daemon`, `prepare_for_push`, and now `run`) into registered repositories so `.tools\run` works from any registered repository as well as from `work_dot_bat`.
- `src/internal/projects/projects.go` manages `projects.json` (`Project` struct, `ValidateRegistry`, `LoadRegistry`, `SaveRegistry`, `HasConfig`, `ParseCommandLine`, `FormatCommand`). `ExtraCheck` already uses `{ Name string; Command Command }` — `LaunchCommand` can follow the exact same shape and share validation.
- `src/internal/menu/menu.go` uses `Ask`/`Log` callbacks for testable interactive menus.
- `src/cmd/work_dot_bat/main.go` resolves `project=<name>` or runs the interactive project menu, activates the project (`repo.Activate`), and dispatches to tools registered in `src/internal/tools/tools.go`.

# Context
- `src/internal/projects/projects.go` — add `LaunchCommand` and `Project.LaunchCommands` (`launchCommands` in `projects.json`).
- `src/internal/launcher/launcher.go` — new package for the interactive launch menu, adding new entries to `projects.json`, and executing the chosen command.
- `src/cmd/work_dot_bat/run.go` — registers the `"run"` tool with `tools.Register("run", ...)` and wires `main.go` / `tools.Names`.
- `src/internal/wrappers/wrappers.go` — include `"run"` in `wrappers.Tools`.
- `.tools/run` & `.tools/run.bat` — entry-point scripts calling `work --run run`.

# Implementation Plan
1. Extend `projects.Project` with `LaunchCommands []LaunchCommand` (`json:"launchCommands,omitempty"`), where `LaunchCommand` has `Name string` (`json:"name"`) and `Command Command` (`json:"command"`). Share named-command validation with `extraChecks` in `ValidateRegistry`.
2. Create `src/internal/launcher/launcher.go` with `Options` (`Ask`, `Log`, `Project`, `RegistryFile`, `Exec`) and `Run(opts Options) int`:
   - Displays numbered launch commands (`1..N`) with their name and formatted command, plus `a) Add a new launch command` and `q) Quit`.
   - Selecting `1..N` executes the command in `Project.Path` via `Exec` and returns its exit code.
   - Selecting `a` prompts for `Command: ` first, then `Description: `, appends the `LaunchCommand` to the project in `RegistryFile` via `LoadRegistry` / `SaveRegistry`, and returns to the menu.
   - Selecting `q` or EOF exits with 0.
3. Register `"run"` in `src/internal/tools/tools.go` (`Names`) and implement `src/cmd/work_dot_bat/run.go`.
4. Add `"run"` to `wrappers.Tools` and create `.tools/run` and `.tools/run.bat`.

# Testing Plan
- `src/internal/projects/projects_test.go`: test `launchCommands` serialization, `HasConfig()`, and `ValidateRegistry` (valid entries and malformed entries).
- `src/internal/launcher/launcher_test.go`: test menu rendering, selecting a numbered command (verifying command and working directory passed to `Exec` and exit code propagation), adding a new entry (`a` -> command -> description -> persisted to `projects.json` -> selectable in menu), invalid choices, empty input handling, and quitting (`q` / EOF).
- `src/internal/wrappers/wrappers_test.go`: verify `run` and `run.bat` wrappers are generated.
- `src/cmd/work_dot_bat/main_test.go`: verify `work_dot_bat run project=<name>` dispatches to the registered `run` tool.
