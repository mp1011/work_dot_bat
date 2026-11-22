// Package launcher implements the universal application launcher menu (`run` tool).
package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"work_dot_bat/src/internal/projects"
)

// Options configures an interactive launcher session.
type Options struct {
	Ask          func(prompt string) (string, bool)
	Log          func(msg string)
	Project      projects.Project
	RegistryFile string
	Exec         func(cmd projects.Command, dir string) int
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
}

// FormatMenu renders the numbered launch menu for a project.
func FormatMenu(projectName string, commands []projects.LaunchCommand) string {
	width := 0
	for _, c := range commands {
		if len(c.Name) > width {
			width = len(c.Name)
		}
	}
	lines := []string{fmt.Sprintf("Select a launch mode for %s:", projectName)}
	if len(commands) == 0 {
		lines = append(lines, "  (no launch commands configured)")
	} else {
		for i, c := range commands {
			lines = append(lines, fmt.Sprintf("  %d) %-*s   %s", i+1, width, c.Name, projects.FormatCommand(c.Command)))
		}
	}
	lines = append(lines, "  a) Add a new launch command", "  q) Quit")
	return strings.Join(lines, "\n")
}

// Run displays the launch menu for opts.Project, handles adding new entries,
// and executes the selected launch command.
func Run(opts Options) int {
	log := opts.Log
	if log == nil {
		log = func(string) {}
	}
	execFn := opts.Exec
	if execFn == nil {
		execFn = func(cmd projects.Command, dir string) int {
			return defaultExec(cmd, dir, opts.Stdin, opts.Stdout, opts.Stderr)
		}
	}

	proj := opts.Project
	for {
		log(FormatMenu(proj.Name, proj.LaunchCommands))
		raw, ok := opts.Ask("Choice: ")
		if !ok {
			return 0
		}
		choice := strings.TrimSpace(raw)
		if strings.EqualFold(choice, "q") {
			return 0
		}
		if strings.EqualFold(choice, "a") {
			updated, exitLoop := addLaunchCommand(opts, proj, log)
			if exitLoop {
				return 0
			}
			proj = updated
			continue
		}
		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(proj.LaunchCommands) {
			log("Invalid choice.")
			continue
		}
		return execFn(proj.LaunchCommands[idx-1].Command, proj.Path)
	}
}

func addLaunchCommand(opts Options, proj projects.Project, log func(string)) (projects.Project, bool) {
	rawCmd, ok := opts.Ask("  Command: ")
	if !ok {
		return proj, true
	}
	cmd := projects.ParseCommandLine(strings.TrimSpace(rawCmd))
	if cmd == nil {
		log("Command cannot be empty.")
		return proj, false
	}

	rawDesc, ok := opts.Ask("  Description: ")
	if !ok {
		return proj, true
	}
	desc := strings.TrimSpace(rawDesc)
	if desc == "" {
		log("Description cannot be empty.")
		return proj, false
	}

	entry := projects.LaunchCommand{Name: desc, Command: *cmd}
	if opts.RegistryFile != "" {
		list, _, err := projects.LoadRegistry(opts.RegistryFile)
		if err != nil {
			log(fmt.Sprintf("Failed to load registry: %v", err))
			return proj, false
		}
		updated := false
		for i := range list {
			if list[i].Name == proj.Name || projects.SamePath(list[i].Path, proj.Path) {
				list[i].LaunchCommands = append(list[i].LaunchCommands, entry)
				proj.LaunchCommands = list[i].LaunchCommands
				updated = true
				break
			}
		}
		if !updated {
			proj.LaunchCommands = append(proj.LaunchCommands, entry)
			list = append(list, proj)
		}
		if err := projects.SaveRegistry(opts.RegistryFile, list); err != nil {
			log(fmt.Sprintf("Failed to save registry: %v", err))
			return proj, false
		}
	} else {
		proj.LaunchCommands = append(proj.LaunchCommands, entry)
	}
	log(fmt.Sprintf("Added %q (%s).", desc, projects.FormatCommand(*cmd)))
	return proj, false
}

func resolveCommand(goos string, c projects.Command, findBash func() string) (string, []string) {
	if c.Shell != nil && *c.Shell {
		line := projects.FormatCommand(c)
		if goos == "windows" {
			return "cmd", []string{"/c", line}
		}
		return "sh", []string{"-c", line}
	}
	if goos == "windows" {
		lower := strings.ToLower(c.Cmd)
		if strings.HasSuffix(lower, ".sh") {
			bash := "bash"
			if findBash != nil {
				if b := findBash(); b != "" {
					bash = b
				}
			}
			return bash, append([]string{c.Cmd}, c.Args...)
		}
		if strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd") {
			return filepath.FromSlash(c.Cmd), c.Args
		}
	}
	return c.Cmd, c.Args
}

func findWindowsBash() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Programs", "Git", "bin", "bash.exe"),
	}
	for _, p := range candidates {
		if p != "" && p != string(filepath.Separator) {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	if p, err := osexec.LookPath("bash"); err == nil {
		return p
	}
	return "bash"
}

func defaultExec(c projects.Command, dir string, stdin io.Reader, stdout, stderr io.Writer) int {
	name, args := resolveCommand(runtime.GOOS, c, findWindowsBash)
	cmd := osexec.Command(name, args...)
	cmd.Dir = dir
	if len(c.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range c.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdin = os.Stdin
	if stdin != nil {
		cmd.Stdin = stdin
	}
	cmd.Stdout = os.Stdout
	if stdout != nil {
		cmd.Stdout = stdout
	}
	cmd.Stderr = os.Stderr
	if stderr != nil {
		cmd.Stderr = stderr
	}
	if err := cmd.Run(); err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(cmd.Stderr, "run: failed to execute %s: %v\n", c.Cmd, err)
		return 1
	}
	return 0
}
