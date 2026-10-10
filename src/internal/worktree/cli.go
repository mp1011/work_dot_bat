package worktree

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"work_dot_bat/internal/menu"
	"work_dot_bat/internal/planstatus"
	"work_dot_bat/internal/taskmenu"
)

// App is the create_worktree tool. Two modes: name the branch yourself (no bootstrap), or
// derive it from a plan (named after the plan, or its parent for a subtask) and bootstrap the
// new worktree so it can run the automated checks immediately.
type App struct {
	In          *bufio.Reader
	Out, Err    io.Writer
	Run         Runner
	RepoRoot    string
	ProjectName string
	Provision   func(treeRoot, projectRoot string) (Provisioning, error)
	// Open launches the editor on a worktree; nil skips it.
	Open func(path string)
	// PickPlan returns the chosen plan path, or "" for none. Nil uses a numbered list of
	// the open backlog.
	PickPlan func() string
}

func (a App) say(format string, args ...any) { fmt.Fprintf(a.Out, format+"\n", args...) }
func (a App) log(msg string)                 { fmt.Fprintln(a.Out, msg) }

func (a App) ask(prompt string) (string, bool) {
	fmt.Fprint(a.Out, prompt)
	line, err := a.In.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

func (a App) open(path string) {
	if a.Open != nil {
		a.say("Launching VS Code...")
		a.Open(path)
	}
}

// Main runs the tool. Optional arguments skip the prompts: branch=<name> or plan=<path>.
func (a App) Main(args []string) int {
	branch, plan := "", ""
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "branch="):
			branch = strings.TrimPrefix(arg, "branch=")
		case strings.HasPrefix(arg, "plan="):
			plan = strings.TrimPrefix(arg, "plan=")
		}
	}
	a.say("===================================================")
	a.say("              Git Worktree Helper")
	a.say("===================================================")
	a.say("Repo Root: %s\n", a.RepoRoot)

	switch {
	case branch != "":
		return a.manual(branch)
	case plan != "":
		return a.plan(planstatus.NormalizePlanPath(plan))
	}
	a.say("  1) Name the branch myself")
	a.say("  2) Derive the branch from a plan file\n")
	for {
		answer, ok := a.ask("Choose option (1/2): ")
		if !ok {
			return 0
		}
		switch strings.ToLower(answer) {
		case "1":
			name, _ := a.ask("Enter git branch name: ")
			return a.manual(name)
		case "2":
			return a.plan("")
		case "q", "quit":
			a.say("Exiting without changes.")
			return 0
		}
		a.say("Invalid selection. Enter 1, 2, or q to quit.")
	}
}

func (a App) manual(branch string) int {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		a.say("[ERROR] Branch name cannot be empty.")
		return 1
	}
	target := PathFor(a.RepoRoot, branch, a.ProjectName)
	if _, err := os.Stat(target); err == nil {
		// The fast path for someone about to open the folder anyway: deliberately no bootstrap.
		a.say("[WARNING] Directory \"%s\" already exists.", target)
		if ans, _ := a.ask("Would you like to open it in VS Code anyway? (y/n): "); strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes") {
			a.open(target)
		}
		return 0
	}
	res, err := Ensure(a.RepoRoot, branch, a.ProjectName, a.Run, a.log)
	if err != nil {
		a.say("[ERROR] %v", err)
		return 1
	}
	if !res.OK {
		return 1
	}
	a.say("Worktree created successfully.")
	a.open(res.Path)
	return 0
}

func (a App) pickPlan() string {
	if a.PickPlan != nil {
		return a.PickPlan()
	}
	m := taskmenu.New(menu.IO{Ask: a.ask, Log: a.log}, a.RepoRoot, isTerminal(a.Out))
	if r := m.Select(); r.Task != nil {
		return r.Task.Path
	}
	return ""
}

func (a App) plan(planPath string) int {
	if planPath == "" {
		planPath = a.pickPlan()
	}
	if planPath == "" {
		a.say("\nNo plan selected. Exiting without changes.")
		return 0
	}
	cli := planstatus.CLI{Root: a.RepoRoot, Collect: planstatus.GitCollectWorktreeStatuses}
	entries := cli.OverlaidEntries()
	var entry planstatus.Entry
	for _, e := range entries {
		if e.Path == planPath {
			entry = e
		}
	}
	branch := planstatus.BranchNameForPlan(planPath, entries)
	a.say("\nPlan:     %s", planPath)
	if entry.SubtaskOf != "" {
		a.say("Subtask of %s", entry.SubtaskOf)
	}
	a.say("Branch:   %s", branch)
	a.say("Worktree: %s\n", PathFor(a.RepoRoot, branch, a.ProjectName))

	res, err := Ensure(a.RepoRoot, branch, a.ProjectName, a.Run, a.log)
	if err != nil {
		a.say("[ERROR] %v", err)
		return 1
	}
	if !res.OK {
		return 1
	}
	if res.Created {
		prov, perr := a.Provision(res.Path, a.RepoRoot)
		if perr != nil || !Bootstrap(res.Path, prov, a.Run, a.log) {
			if perr != nil {
				a.say("[ERROR] %v", perr)
			}
			a.say("\n[ERROR] Worktree was created but bootstrap failed. Fix it before running the automated checks there.")
			return 1
		}
		a.say("Worktree ready.")
	}
	a.open(res.Path)
	return 0
}

// RunInfo implements `worktree-info`: where this checkout sits, as a label/value table.
func RunInfo(cwd string, out, errOut io.Writer, git GitFunc) int {
	info, err := GetInfo(cwd, git)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}
	fmt.Fprintln(out, FormatInfo(info))
	return 0
}

// isTerminal reports whether w is an interactive terminal (a character device).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
