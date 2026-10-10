# Quick Start Guide

This guide covers the commands to build, test, and run `work_dot_bat`. Only scripts and commands intended for humans to run are included.

---

## Prerequisites

- **Go 1.22+** (only the Go toolchain is needed; all dependencies are in the Go standard library or vendored).

---

## 1. Build

### Direct Go Build
Build the entire project and perform static analysis:
```bash
go build ./...
go vet ./...
```

Or build a specific binary locally:
```bash
go build -o dist/work_dot_bat_windows_amd64.exe ./src/cmd/work_dot_bat
```

### Local Build Script
Compiles the Windows amd64 binary into `dist/` for quick local testing:
```bash
# Bash / Git Bash
./.tools/build-local.sh
```

### Cross-Platform Release Build
Cross-compiles standalone binaries for all 6 target platforms (`windows`, `darwin`, `linux` on `amd64` and `arm64`) into `dist/`:
```bash
# Bash / Git Bash
./.tools/build-release.sh [version]
```

---

## 2. Test

### Unit & Package Tests
Run Go unit tests across all packages:
```bash
go test ./...
```

### Smoke Tests
Verify a compiled binary runs and responds across basic subcommands:
```bash
# Bash / Git Bash
./.tools/smoke.sh dist/work_dot_bat_windows_amd64.exe
```

### Full Mechanical Checks Gate
Runs the full suite of mechanical checks, project structure validations, builds, and test suites (what CI and pre-push hooks execute):
```bash
# Bash / Git Bash
./.tools/run-checks.sh

# Or via work_dot_bat runner
.\.tools\work.bat --run run-checks
```

---

## 3. Run

### (Optional) Add `.tools` to PATH
To run the commands without specifying the `.tools` path:
- **PowerShell**:
  ```powershell
  .\.tools\add-to-path.ps1
  ```
- **Command Prompt (CMD)**:
  ```cmd
  .\.tools\add-to-path.bat
  ```
- **Bash / Linux / macOS**:
  ```bash
  ./.tools/add-to-path.sh
  ```

---

### Human-Facing Workflow Scripts

The scripts in `.tools/` automatically locate the compiled binary in `dist/`, on your `PATH`, or via `WORK_DOT_BAT_BIN`.

#### Interactive Task Launcher (`work`)
Presents an interactive menu to choose a project, select an agent plan, provision worktrees, and launch tasks:
- **Windows**:
  ```cmd
  .\.tools\work.bat [project=<name>]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/work [project=<name>]
  ```

#### Pre-Push Verification (`prepare_for_push`)
Runs mechanical checks and guides human verification before committing and pushing:
- **Windows**:
  ```cmd
  .\.tools\prepare_for_push.bat [project=<name>]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/prepare_for_push [project=<name>]
  ```

#### Autonomous Plan Daemon (`auto_daemon`)
Runs the unattended daemon that picks up plans marked `auto: true` in their own worktrees:
- **Windows**:
  ```cmd
  .\.tools\auto_daemon.bat [project=<name>]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/auto_daemon [project=<name>]
  ```

#### Daemon Status & Plan Queue (`auto_status`)
Reports which auto-plans are ready to be picked up and which are blocked or held back:
- **Windows**:
  ```cmd
  .\.tools\auto_status.bat [project=<name>]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/auto_status [project=<name>]
  ```

#### Worktree Provisioning (`create_worktree`)
Provisions an isolated Git worktree for a specific plan:
- **Windows**:
  ```cmd
  .\.tools\create_worktree.bat [project=<name>] <plan-path>
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/create_worktree [project=<name>] <plan-path>
  ```

#### Unified CLI Runner (`work_dot_bat`)
Pass any subcommand and options directly to the underlying `work_dot_bat` executable:
- **Windows**:
  ```cmd
  .\.tools\work.bat --run <subcommand> [args]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/work --run <subcommand> [args]
  ```

#### Install Repo-Local Tool Wrappers (`install-wrappers`)
Writes/updates local `.tools` launchers (`work.bat`, `auto_daemon.bat`, `prepare_for_push.bat`, and bash scripts) directly inside target project repositories. This allows launching workflows directly from a project's repository (e.g. `.\.tools\work.bat`):
- **Windows**:
  ```cmd
  .\.tools\work.bat --run install-wrappers [project=<name>]
  ```
- **POSIX / Bash**:
  ```bash
  ./.tools/work --run install-wrappers [project=<name>]
  ```
*(Omit `project=<name>` to install wrappers into all registered project repositories).*

