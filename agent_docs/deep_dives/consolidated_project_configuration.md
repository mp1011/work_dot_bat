# Consolidated Project Configuration

## Overview
Previously, `work_dot_bat` configured project-level commands (`build`, `install`, `devServer`, `checks`, `extraChecks`, `worktreeCopyFiles`, etc.) inside a per-repository `work_dot_bat.config.json` file, while project path mappings were stored in the tool-level `projects.json` registry.

With configuration consolidation, `projects.json` now serves as the single centralized source of truth for each registered project. All build, execution, testing, and provisioning settings live directly within each project's registry entry in `projects.json`.

## Schema in projects.json
Each project entry in `projects.json` supports:
- `name` (string, required): Unique project identifier.
- `path` (string, required): Absolute or resolved repository root path.
- `build` (array of commands, optional): List of build commands executed sequentially during build/check cycles.
- `install` (command, optional): Package/dependency installation command run when provisioning fresh worktrees.
- `devServer` (command or null, optional): Command to start the development server.
- `tests` (object, optional): Four test tiers (`ultra_light`, `light`, `heavy`, `ultra_heavy`).
- `checks` (array of strings, optional): Enabled tool-owned check IDs (e.g. `monolith`, `duplication`, `skills`).
- `extraChecks` (array of objects, optional): Extra custom checks with `{ name, command }`.
- `worktreeCopyFiles` (array of strings, optional): Machine-local untracked files copied to new worktrees.
- `projectStructureDoc` (string, optional): Repo-relative path to project structure documentation.
- `projectStructureDirs` (array of strings, optional): Folders scanned for project structure generation.
- `agents` (object, optional): Agent CLI configuration overrides (Claude, Antigravity).
- `planTemplate` (string, optional): Repo-relative path to custom plan template.
- `instructions` (object, optional): Custom instructions configuration (such as `daemonDoc`).

Command entries can be specified either as objects with `cmd` and `args` (e.g. `{"cmd": "npm", "args": ["install"]}`) or as concise string arrays (e.g. `["npm", "install"]`).

## Resolution Precedence
Config resolution (`projectconfig.ForProject`) resolves configuration in the following order:
1. Matched project in `projects.json`: If the project is registered in `projects.json` and defines configuration fields (`HasConfig()`), those settings are used directly.
2. Backwards-compatible fallback: If the project entry lacks settings in `projects.json` (or is not yet registered), `ForProject` falls back to reading the legacy `<repo>/work_dot_bat.config.json`.

## Workflows
- **Interactive Registration (`internal/menu`)**: Registering a new project via the interactive menu (`p` in `work`) prompts for project commands and writes them directly into `projects.json` without creating `work_dot_bat.config.json`.
- **Bootstrap Tool (`internal/bootstrap`)**: Bootstrapping a repository (`work_dot_bat bootstrap`) writes the project's configuration into `projects.json` rather than creating `work_dot_bat.config.json`.
- **Worktree Provisioning (`internal/worktree`)**: Worktree provisioning resolves `install`, `devServer`, and `worktreeCopyFiles` via `projectconfig.ForProject`, executing commands directly from `projects.json`.
