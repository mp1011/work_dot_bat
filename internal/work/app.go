package work

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"work_dot_bat/internal/agentcmd"
	"work_dot_bat/internal/agentsession"
	"work_dot_bat/internal/batch"
	"work_dot_bat/internal/menu"
	"work_dot_bat/internal/planclaims"
	"work_dot_bat/internal/planstatus"
	"work_dot_bat/internal/taskmenu"
	"work_dot_bat/internal/worktree"
)

// App is the interactive launcher over one project. Fields marked "seam" have working defaults
// from New; tests replace them. RunTests is wired by the caller (the gate lives in package checks);
// Loop, Push and MergeBack override the built-in flows.
type App struct {
	Root        string
	ProjectName string
	Out         io.Writer
	// Ask prints a prompt and returns the next input line; ok is false at end of input.
	Ask   func(prompt string) (string, bool)
	IsTTY bool

	ClaimEnv planclaims.Env
	Dirs     agentsession.Dirs

	Now       func() time.Time                           // seam
	Settings  func() agentcmd.Settings                   // seam
	RunAgent  AgentRunner                                // seam
	Git       worktree.Runner                            // seam: git commands, inherited stdio
	Tasks     func() []planstatus.Entry                  // seam: every plan overlaid with worktree status
	Worktrees func() []planstatus.Worktree               // seam
	GitOut    worktree.GitFunc                           // seam: git reads; nil = real git
	Pick      func() taskmenu.Result                     // seam: the Step 1 menu
	SetStatus func(path, status string, human bool) bool // seam
	Guard     func() (stop func())                       // seam: see GuardInterrupt

	// RunTests runs the project's automated checks and returns the failing check names.
	RunTests  func() []string
	MergeBack func(task *taskmenu.Item)
	Loop      func(ctx context.Context, parent *taskmenu.Item, agent string) (success bool)
	Push      func(agent string) (newAgent string, pushed bool)
}

func (a App) logf(format string, args ...any) { fmt.Fprintf(a.Out, format+"\n", args...) }
func (a App) log(msg string)                  { fmt.Fprintln(a.Out, msg) }

func (a App) io() menu.IO { return menu.IO{Ask: a.Ask, Log: a.log} }

func (a App) ask(prompt string) (string, bool) {
	line, ok := a.Ask(prompt)
	return strings.TrimSpace(line), ok
}

// confirm asks a y/n question; anything but y/yes (or end of input) is no.
func (a App) confirm(prompt string) bool {
	ans, _ := a.ask(prompt)
	ans = strings.ToLower(ans)
	return ans == "y" || ans == "yes"
}

func (a App) claims() planclaims.CLI {
	return planclaims.CLI{Root: a.Root, Stdout: a.Out, Stderr: a.Out, Env: a.ClaimEnv}
}

func (a App) guard() func() {
	if a.Guard != nil {
		return a.Guard()
	}
	return GuardInterrupt()
}

func (a App) settings() agentcmd.Settings {
	if a.Settings != nil {
		return a.Settings()
	}
	return agentcmd.ProjectSettings(a.Root, a.Root)
}

func (a App) runAgent() AgentRunner {
	if a.RunAgent != nil {
		return a.RunAgent
	}
	return ExecAgent
}

func (a App) git(args ...string) int {
	run := a.Git
	if run == nil {
		run = worktree.RunCommand
	}
	code, err := run("git", args, worktree.RunOpts{Dir: a.Root})
	if err != nil {
		a.logf("[ERROR] Failed to execute git: %v", err)
		return 1
	}
	return code
}

func (a App) tasks() []planstatus.Entry {
	if a.Tasks != nil {
		return a.Tasks()
	}
	return planstatus.CLI{Root: a.Root, Collect: planstatus.GitCollectWorktreeStatuses}.OverlaidEntries()
}

func (a App) worktrees() []planstatus.Worktree {
	if a.Worktrees != nil {
		return a.Worktrees()
	}
	wts, _ := worktree.ExistingWorktrees(a.Root)
	return wts
}

func (a App) setStatus(path, status string, human bool) bool {
	if a.SetStatus != nil {
		return a.SetStatus(path, status, human)
	}
	if err := planstatus.Set(a.Root, path, status, human); err != nil {
		a.log(err.Error())
		return false
	}
	a.logf("Set %s -> %s", path, status)
	return true
}

func (a App) pick() taskmenu.Result {
	if a.Pick != nil {
		return a.Pick()
	}
	return taskmenu.New(a.io(), a.Root, a.IsTTY).Select()
}

// agentDocsPath is the plan path as the agent and git see it, from the repo root.
func agentDocsPath(p string) string {
	if strings.HasPrefix(p, "agent_docs/") || strings.HasPrefix(p, `agent_docs\`) {
		return p
	}
	return "agent_docs/" + p
}

func toItems(entries []planstatus.Entry) []taskmenu.Item {
	items := make([]taskmenu.Item, len(entries))
	for i, e := range entries {
		items[i] = taskmenu.Item{Entry: e}
	}
	return items
}

// FormatWorktreeSummary is one "- [branch] path" line per worktree.
func FormatWorktreeSummary(worktrees []planstatus.Worktree, repoRoot string) []string {
	norm := strings.TrimRight(strings.ReplaceAll(repoRoot, `\`, "/"), "/")
	lines := make([]string, len(worktrees))
	for i, w := range worktrees {
		p := strings.TrimRight(strings.ReplaceAll(w.Path, `\`, "/"), "/")
		tag, prunable := "", ""
		if w.Branch == "main" || (norm != "" && p == norm) {
			tag = " (main repository)"
		}
		if w.Prunable {
			prunable = " [prunable]"
		}
		lines[i] = fmt.Sprintf("- [%s] %s%s%s", w.Branch, w.Path, tag, prunable)
	}
	return lines
}

type flow int

const (
	stay flow = iota
	back      // return to task selection
	quit
)

// Run is the launcher's main loop; it returns the process exit code.
func (a App) Run(ctx context.Context) int {
	a.ClaimEnv.PruneStaleClaims(a.Root, a.log)
	for {
		a.log("\n===================================================")
		a.log("              Agent Work Helper")
		a.log("===================================================")
		if trees := a.worktrees(); len(trees) > 0 {
			if lines := FormatWorktreeSummary(trees, a.Root); len(lines) > 0 {
				a.logf("\nActive Git Worktrees (%d):", len(trees))
				for _, l := range lines {
					a.log("  " + l)
				}
			}
		}
		res := a.pick()
		if res.Quit {
			return 0
		}
		agent := ""
		f := stay
		for f == stay {
			if res.Task != nil {
				f = a.taskStep(ctx, res.Task, &agent)
			} else {
				f = a.noTaskStep(ctx, &agent)
			}
		}
		if f == quit {
			return 0
		}
	}
}

func printOptions(a App, actions, nav []Option) string {
	var keys []string
	for _, o := range actions {
		a.logf("  %s) %s", o.Key, o.Label)
		keys = append(keys, o.Key)
	}
	a.log("")
	for _, o := range nav {
		a.logf("  %s) %s", o.Key, o.Label)
		keys = append(keys, o.Key)
	}
	a.log("")
	return strings.Join(keys, "/")
}

func (a App) noTaskStep(ctx context.Context, agent *string) flow {
	a.log("\n---------------------------------------------------")
	a.log("Task: None selected")
	a.log("---------------------------------------------------")
	actions, nav := TaskMenuOptions(nil, false, false)
	keys := printOptions(a, actions, nav)
	opt, ok := a.ask(fmt.Sprintf("Select option (%s): ", keys))
	if !ok {
		return quit
	}
	switch opt = strings.ToLower(opt); opt {
	case "c":
		a.commitWorkflow()
	case "p":
		push := a.Push
		if push == nil {
			push = func(agent string) (string, bool) { return a.pushWorkflow(ctx, agent) }
		}
		newAgent, pushed := push(*agent)
		*agent = newAgent
		if pushed {
			return back
		}
	case "t":
		a.runTests()
	case "s":
		a.log("\nReturning to task selection...")
		return back
	case "q", "exit":
		a.log("\nExiting work script.")
		return quit
	default:
		a.logf("\nInvalid option \"%s\". Please select one of %s.", opt, keys)
	}
	return stay
}

func (a App) runTests() {
	if a.RunTests == nil {
		a.log("\n[NOT YET PORTED] The automated test runner is not available in the Go launcher yet.")
		return
	}
	a.RunTests()
}

// loopUntilDone is loop mode: every eligible subtask of the parent, then the parent, run
// non-interactively under the launcher's claim. It needs the automated test runner; without
// one it refuses rather than call unverified work done.
func (a App) loopUntilDone(ctx context.Context, parent *taskmenu.Item, agent string) bool {
	if a.RunTests == nil {
		a.log("\n[NOT YET PORTED] Loop mode needs the automated test runner, which is not available in the Go launcher yet.")
		return false
	}
	stop := a.guard()
	defer stop()
	res := batch.RunLoopUntilDone(batch.LoopRun{
		Parent:   parent.Entry,
		GetTasks: a.tasks,
		RunAgent: func(prompt, planPath string, attempt int) batch.AgentResult {
			return a.RunAgentNonInteractive(ctx, agent, prompt, planPath, attempt)
		},
		RunTests:     a.RunTests,
		AcquireClaim: a.AcquireLeafClaim,
		ReleaseClaim: a.ReleaseLeafClaim,
		ReadPlan: func(planPath string) string {
			full := planPath
			if !filepath.IsAbs(full) {
				full = filepath.Join(a.Root, filepath.FromSlash(planPath))
			}
			b, _ := os.ReadFile(full)
			return string(b)
		},
		UpdateStatus: func(path, status string) bool { return a.setStatus(path, status, false) },
		OnLog:        a.log,
	})
	return res.Success
}

func (a App) promptRunTests() {
	if a.confirm("\nDo you want to run automated tests? (y/n): ") {
		a.runTests()
	}
}

func (a App) chooseAgent(current *string) (ok bool) {
	agent, quit, more := agentcmd.ChooseAgent(*current, a.io())
	if quit || !more {
		return false
	}
	*current = agent
	return true
}

func (a App) markInProgress(task *taskmenu.Item) {
	if task.Status == "Not Started" || task.Status == "Suggestion" {
		a.logf("\nUpdating task status from \"%s\" to \"In Progress\"...", task.Status)
		if a.setStatus(task.Path, "In Progress", false) {
			task.Status = "In Progress"
		}
	}
}

func (a App) taskStep(ctx context.Context, task *taskmenu.Item, agent *string) flow {
	tasks := a.tasks()
	for _, t := range tasks {
		if t.Path == task.Path {
			task.Status, task.Auto = t.Status, t.Auto
			break
		}
	}
	items := toItems(tasks)
	autoTag := ""
	if taskmenu.IsAutoTask(task.Entry, items) {
		autoTag = " [auto]"
	}
	a.log("\n---------------------------------------------------")
	a.logf("Task: %s%s", task.Path, autoTag)
	a.logf("Current Status: [%s]", taskmenu.ColorizeStatus(task.Status, task.Status, a.IsTTY))
	branch := planstatus.BranchNameForPlan(task.Path, tasks)
	wt := worktree.FindForBranch(a.worktrees(), branch)
	if wt != nil {
		a.logf("Worktree: %s (%s)", wt.Path, branch)
	}
	a.log("---------------------------------------------------")

	canWork := task.Status != "Done" && task.Status != "Cancelled"
	isParent := taskmenu.TaskHasSubtasks(task.Entry, items)
	actions, nav := TaskMenuOptions(&task.Entry, wt != nil, isParent)
	keys := printOptions(a, actions, nav)
	opt, ok := a.ask(fmt.Sprintf("Select option (%s): ", keys))
	if !ok {
		return quit
	}
	opt = strings.ToLower(opt)
	if opt == "x" {
		opt = "c"
	}
	switch opt {
	case "w":
		if !canWork {
			a.logf("\nTask is marked %s.", task.Status)
			break
		}
		return a.workOnTask(ctx, task, agent, isParent)
	case "l":
		switch {
		case !isParent:
			a.log("\nTask does not have subtasks.")
		case !canWork:
			a.logf("\nTask is marked %s.", task.Status)
		case !a.chooseAgent(agent):
			return quit
		default:
			loop := a.Loop
			if loop == nil {
				loop = a.loopUntilDone
			}
			if loop(ctx, task, *agent) {
				task.Status = "Ready for Review"
			}
		}
	case "g":
		if !canWork {
			a.logf("\nTask is marked %s.", task.Status)
			break
		}
		return a.generalRequest(ctx, task, agent)
	case "d":
		if task.Status != "Ready for Review" {
			a.log("\nTask must be \"Ready for Review\" to mark as Done.")
			break
		}
		a.logf("\nMarking \"%s\" as Done...", task.Path)
		if a.setStatus(task.Path, "Done", false) {
			task.Status = "Done"
			a.log("Task successfully marked as Done!")
		}
	case "c":
		a.cancelTask(task)
	case "m":
		if a.MergeBack == nil {
			a.log("\n[NOT YET PORTED] Merge back is not available in the Go launcher yet.")
		} else {
			a.MergeBack(task)
		}
	case "t":
		a.runTests()
	case "a":
		switch {
		case task.Auto:
			a.logf("\n\"%s\" is already marked auto: true.", task.Path)
		case !canWork:
			a.logf("\nTask is marked %s.", task.Status)
		default:
			a.setAutoWorkflow(task)
		}
	case "s":
		a.log("\nReturning to task selection...")
		return back
	case "q", "exit":
		a.log("\nExiting work script.")
		return quit
	default:
		a.logf("\nInvalid option \"%s\". Please select one of %s.", opt, keys)
	}
	return stay
}

// workOnTask launches the work skill on the task. A leaf plan holds the plan/branch claim for the
// whole run (acquired here, released by defer) instead of leaving the spawned agent to acquire
// and maybe not release it; a parent plan spawns next-subtask-yolo, which acquires the
// *subtask's* claim itself, so this launcher must not take the shared branch scope from under it.
func (a App) workOnTask(ctx context.Context, task *taskmenu.Item, agent *string, isParent bool) flow {
	a.markInProgress(task)
	special, ok := a.ask("\nEnter any special instructions for this task (or press Enter to skip): ")
	if !ok || !a.chooseAgent(agent) {
		return quit
	}
	planPath := agentDocsPath(task.Path)
	skill := "do-work-yolo"
	if isParent {
		skill = "next-subtask-yolo"
	}
	prompt := "/" + skill + " " + planPath
	if special != "" {
		prompt += " " + special
	}
	launch := func(prompt string) {
		if err := a.LaunchInteractiveAgent(ctx, *agent, planPath, prompt); err != nil {
			a.logf("\n[ERROR] %v", err)
			return
		}
		a.promptRunTests()
	}
	if isParent {
		launch(prompt)
		return stay
	}
	if !a.WithClaim(planPath, func() { launch(prompt + agentcmd.LauncherClaimNote) }) {
		a.log("\n[SKIP] The plan/branch claim is held by someone else. Not starting this task.")
	}
	return stay
}

func (a App) generalRequest(ctx context.Context, task *taskmenu.Item, agent *string) flow {
	a.markInProgress(task)
	request, ok := a.ask("\nEnter your request for the agent (or press Enter to skip): ")
	if !ok || !a.chooseAgent(agent) {
		return quit
	}
	planPath := agentDocsPath(task.Path)
	prompt := agentcmd.BuildGeneralAgentPrompt(planPath, request, a.Root, a.log)
	if err := a.LaunchInteractiveAgent(ctx, *agent, planPath, prompt); err != nil {
		a.logf("\n[ERROR] %v", err)
		return stay
	}
	a.promptRunTests()
	return stay
}

func (a App) cancelTask(task *taskmenu.Item) {
	if task.Status == "Done" || task.Status == "Cancelled" {
		a.logf("\nTask is already marked %s.", task.Status)
		return
	}
	justification, _ := a.ask("\nEnter justification for cancelling this task: ")
	if justification == "" {
		a.log("Justification cannot be empty. Task cancellation cancelled.")
		return
	}
	a.logf("\nMarking \"%s\" as Cancelled...", task.Path)
	if !a.setStatus(task.Path, "Cancelled", true) {
		return
	}
	task.Status = "Cancelled"
	full := filepath.Join(a.Root, filepath.FromSlash(agentDocsPath(task.Path)))
	existing, err := os.ReadFile(full)
	switch {
	case os.IsNotExist(err):
		a.logf("[WARN] Plan file does not exist at %s", full)
	case err != nil:
		a.logf("[ERROR] Failed to update plan file with justification: %v", err)
	default:
		content := string(existing)
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n# CANCELLED\nJustification: " + justification + "\n"
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			a.logf("[ERROR] Failed to update plan file with justification: %v", err)
		} else {
			a.log("Updated plan file with cancellation justification.")
		}
	}
	a.log("Task successfully marked as Cancelled!")
}

func (a App) commitWorkflow() {
	msg, _ := a.ask("\nEnter commit message: ")
	if msg == "" {
		a.log("Commit message cannot be empty. Commit cancelled.")
		return
	}
	a.log("\nStaging all files (git add -A)...")
	a.git("add", "-A")
	a.logf("Running git commit -m \"%s\"...", msg)
	a.git("commit", "-m", msg)
}

// setAutoWorkflow sets `auto: true` on the plan and commits just that file so the Auto Daemon
// picks it up. The one human-only exception to "no writer for auto"; it only runs after an
// explicit confirmation.
func (a App) setAutoWorkflow(task *taskmenu.Item) {
	if task.Auto {
		a.logf("\n\"%s\" is already marked auto: true.", task.Path)
		return
	}
	planPath := agentDocsPath(task.Path)
	full := filepath.Join(a.Root, filepath.FromSlash(planPath))
	if _, err := os.Stat(full); err != nil {
		a.logf("\n[ERROR] Plan file does not exist: %s", planPath)
		return
	}
	a.logf("\nThis marks \"%s\" auto: true, then commits just this", task.Path)
	a.log("file so the Auto Daemon can pick it up. This is a human-only action.")
	if !a.confirm("\nProceed? (y/n): ") {
		a.log("Cancelled. Nothing was changed.")
		return
	}
	b, err := os.ReadFile(full)
	if err != nil {
		a.logf("\n[ERROR] Could not read plan file: %v", err)
		return
	}
	content := string(b)
	updated, err := InsertAutoFlag(content)
	if err != nil {
		a.logf("\n[ERROR] %v", err)
		return
	}
	if updated == content {
		a.logf("\n\"%s\" already carries auto: true.", task.Path)
		task.Auto = true
		return
	}
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		a.logf("\n[ERROR] Could not write plan file: %v", err)
		return
	}
	a.logf("\nWrote auto: true to %s.", planPath)
	a.log("Staging and committing this plan file...")
	outcome := CommitAutoFlag(planPath, func(_ string, args ...string) int { return a.git(args...) })
	if outcome == "add-failed" || outcome == "commit-failed" {
		a.log("\n[ERROR] git commit failed. auto: true was written to disk but not committed.")
		return
	}
	task.Auto = true
	a.logf("\n✅ \"%s\" is now auto: true and committed.", task.Path)
}
