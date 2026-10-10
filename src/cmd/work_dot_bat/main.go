// Launcher for every tool: work_dot_bat <tool> [project=<name>] [tool args...]
// Resolves the project (argument or menu), makes it the process's project, then runs the
// tool's entry point from the tools registry.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/tools"
	"work_dot_bat/src/internal/worktree"
	"work_dot_bat/src/internal/wrappers"
)

// Version is stamped at release build time: -ldflags "-X main.Version=1.2.3".
var Version = "dev"

type env struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool
	registry    string
}

func main() {
	st, err := os.Stdin.Stat()
	interactive := err == nil && st.Mode()&os.ModeCharDevice != 0
	os.Exit(run(os.Args[1:], env{os.Stdin, os.Stdout, os.Stderr, interactive, projects.DefaultRegistryFile()}))
}

var (
	activeProject *projects.Project
	activeEnv     *env
)

func run(argv []string, e env) int {
	activeEnv = &e
	defer func() {
		activeEnv = nil
		activeProject = nil
	}()
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(e.stderr, format+"\n", a...)
		return code
	}

	if len(argv) > 0 && (argv[0] == "--version" || argv[0] == "-v" || argv[0] == "version") {
		fmt.Fprintf(e.stdout, "work_dot_bat %s\n", Version)
		return 0
	}
	if len(argv) > 0 {
		if code, ok := runMaintCommand(argv, e); ok {
			return code
		}
	}
	if len(argv) > 0 && argv[0] == "install-wrappers" {
		return runInstallWrappers(argv[1:], e)
	}
	if len(argv) > 0 && argv[0] == "worktree-info" {
		return runWorktreeInfo(e)
	}
	if len(argv) > 0 && argv[0] == "list-tasks" {
		return runPlanStatus(argv, e)
	}
	if len(argv) > 0 && argv[0] == "plan-claim" {
		return runPlanClaim(argv[1:], e)
	}
	if len(argv) > 0 && argv[0] == "plan-status" {
		return runPlanStatus(argv[1:], e)
	}

	if len(argv) == 0 || !tools.Known(argv[0]) {
		return fail(2, "Usage: work_dot_bat <%s> [project=<name>] [args...]", strings.Join(tools.Names, "|"))
	}
	tool := argv[0]
	projectName, rest := projects.ParseProjectArg(argv[1:])

	list, errs, err := projects.LoadRegistry(e.registry)
	if err != nil {
		return fail(1, "%v", err)
	}
	for _, msg := range errs {
		fmt.Fprintf(e.stderr, "WARNING: %s\n", msg)
	}

	var project projects.Project
	if projectName != nil {
		project, err = projects.ResolveProject(list, *projectName)
		if err != nil {
			return fail(1, "%v", err)
		}
	} else {
		// The menu needs a human. An unattended caller (the daemon) must say which project.
		if !e.interactive {
			return fail(1, "No project given and stdin is not interactive. Pass project=<name>. Known projects: %s", projects.KnownNames(list))
		}
		picked := runMenu(list, e)
		if picked == nil {
			return 0
		}
		project = *picked
	}

	if err := repo.Activate(project.Name, project.Path); err != nil {
		return fail(1, "Cannot enter project %s: %v", project.Name, err)
	}
	activeProject = &project
	fmt.Fprintf(e.stdout, "Project: %s (%s)\n", project.Name, project.Path)

	runTool := tools.Lookup(tool)
	if runTool == nil {
		return fail(1, "Tool %q is not yet ported to the Go launcher.", tool)
	}
	return runTool(rest)
}

func runMenu(list []projects.Project, e env) *projects.Project {
	in := bufio.NewReader(e.stdin)
	return menu.Run(menu.Options{
		IO: menu.IO{
			Ask: func(prompt string) (string, bool) {
				fmt.Fprint(e.stdout, prompt)
				line, err := in.ReadString('\n')
				if err != nil && line == "" {
					return "", false
				}
				return strings.TrimRight(line, "\r\n"), true
			},
			Log: func(msg string) { fmt.Fprintln(e.stdout, msg) },
		},
		Projects:     list,
		RegistryFile: e.registry,
	})
}

// Repo-level commands act on the git repository of the working directory, with no project
// registry or "Project:" banner, so their output matches the plan-status.sh / plan-claim.sh
// scripts they replace.
func gitRoot() (string, bool) {
	out, err := osexec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	return filepath.FromSlash(strings.TrimSpace(string(out))), true
}

func runPlanStatus(args []string, e env) int {
	root, ok := gitRoot()
	if !ok {
		fmt.Fprintln(e.stderr, "plan-status: not inside a git repository")
		return 1
	}
	return planstatus.CLI{Root: root, Stdout: e.stdout, Stderr: e.stderr, Collect: planstatus.GitCollectWorktreeStatuses,
		Claims: planclaims.DefaultEnv().ListTaskClaims}.Run(args)
}

func runPlanClaim(args []string, e env) int {
	root, ok := gitRoot()
	if !ok { // the Node CLI falls back to the working directory
		root, _ = os.Getwd()
	}
	return planclaims.CLI{Root: root, Stdout: e.stdout, Stderr: e.stderr, Env: planclaims.DefaultEnv(),
		Collect: planstatus.GitCollectWorktreeStatuses}.Run(args)
}

// runInstallWrappers (re)writes the repo-local .tools wrappers for project=<name>, or for every
// registered project when none is given.
func runInstallWrappers(args []string, e env) int {
	name, _ := projects.ParseProjectArg(args)
	list, _, err := projects.LoadRegistry(e.registry)
	if err != nil {
		fmt.Fprintln(e.stderr, err)
		return 1
	}
	targets := list
	if name != nil {
		p, err := projects.ResolveProject(list, *name)
		if err != nil {
			fmt.Fprintln(e.stderr, err)
			return 1
		}
		targets = []projects.Project{p}
	}
	code := 0
	for _, p := range targets {
		if _, err := wrappers.Install(p.Path, projects.ToolRoot(), p.Name); err != nil {
			fmt.Fprintf(e.stderr, "%s: %v\n", p.Name, err)
			code = 1
			continue
		}
		fmt.Fprintf(e.stdout, "%s: wrote %s wrappers in %s\n", p.Name, wrappers.Dir, p.Path)
	}
	return code
}

func runWorktreeInfo(e env) int {
	cwd, _ := os.Getwd()
	return worktree.RunInfo(cwd, e.stdout, e.stderr, nil)
}

// createWorktree is the create_worktree tool, run in the activated project.
func createWorktree(args []string) int {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return worktree.App{
		In: bufio.NewReader(os.Stdin), Out: os.Stdout, Err: os.Stderr,
		Run: worktree.RunCommand, RepoRoot: filepath.ToSlash(root), ProjectName: os.Getenv(repo.ProjectNameEnv),
		Provision: worktree.LoadProvisioning,
		Open: func(path string) {
			worktree.RunCommand("code", []string{path}, worktree.RunOpts{})
		},
	}.Main(args)
}

func init() { tools.Register("create_worktree", createWorktree) }
