package work

import (
	"fmt"
	"os"
	"path/filepath"

	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/taskmenu"
	"work_dot_bat/src/internal/worktree"
)

func (a App) runner() worktree.Runner {
	if a.Git != nil {
		return a.Git
	}
	return worktree.RunCommand
}

type mergeBack struct {
	branch       string
	worktree     *planstatus.Worktree
	availability worktree.Availability
}

// resolveMergeBack resolves the gate for the selected plan, returning the branch and worktree it
// resolved so the menu and the action agree on exactly one answer.
func (a App) resolveMergeBack(task *taskmenu.Item) mergeBack {
	branch := planstatus.BranchNameForPlan(task.Path, a.tasks())
	wt := worktree.FindForBranch(a.worktrees(), branch)
	return mergeBack{
		branch: branch, worktree: wt,
		availability: worktree.MergeAvailability(worktree.IsTreeDirty(a.GitOut, a.Root), wt, branch, worktree.CurrentBranch(a.GitOut, a.Root)),
	}
}

// mergeBackWorkflow merges the plan's worktree branch into main: gated, shows the commits, stages
// a --no-ff --no-commit merge, reports conflicts without resolving them, and offers safe cleanup.
// It commits only when the user says so, and deletes only with git's non-forcing commands.
func (a App) mergeBackWorkflow(task *taskmenu.Item) {
	m := a.resolveMergeBack(task)
	if !m.availability.Available {
		a.logf("\nCannot merge back: %s", m.availability.Reason)
		return
	}
	branch, wt := m.branch, m.worktree

	// Step 1: uncommitted changes
	mainDirty := worktree.IsTreeDirty(a.GitOut, a.Root)
	wtDirty := worktree.IsTreeDirty(a.GitOut, wt.Path)
	if mainDirty || wtDirty {
		a.log("\nThere are uncommitted changes.")
		if mainDirty {
			a.logf("  - Main branch (%s) has uncommitted changes.", a.Root)
		}
		if wtDirty {
			a.logf("  - Worktree (%s) has uncommitted changes.", wt.Path)
		}
		a.log("Please commit your changes before continuing. (This is a manual action, the tool does not do this.)")
		if !a.confirm("\nDo you want to continue? (y/n): ") {
			a.log("Merge cancelled.")
			return
		}
	}

	// Step 2: already merged or code-equivalent?
	commits := worktree.CommitsToMerge(a.GitOut, a.Root, branch)
	if worktree.IsBranchMerged(a.GitOut, a.Root, branch) || len(commits) == 0 {
		a.logf("\n\"%s\" is already merged into main.", branch)
		a.promptCleanup(branch, wt.Path)
		return
	}

	// Step 3: guard against merging incomplete plans sharing the branch
	if incomplete := worktree.GetIncompleteTasksForBranch(branch, a.tasks()); len(incomplete) > 0 {
		a.logf("\n[WARNING] The following plan(s) sharing branch \"%s\" are still incomplete:", branch)
		for _, t := range incomplete {
			a.logf("  - [%s] %s", t.Status, t.Path)
		}
		if !a.confirm("\nDo you still want to proceed with merging? (y/n): ") {
			a.log("Merge cancelled.")
			return
		}
	}

	a.logf("\nBranch:   %s", branch)
	a.logf("Worktree: %s\n", wt.Path)
	a.logf("Commits to be merged (main..%s):", branch)
	for _, c := range commits {
		a.logf("  %s %s", c.SHA, c.Subject)
	}

	a.log("\nMerging (--no-ff --no-commit)...")
	res := worktree.MergeBranchIntoMain(a.GitOut, a.runner(), a.Root, branch)
	switch res.Outcome {
	case "conflict":
		a.log("\n[CONFLICT] The merge is left in progress. Conflicting paths:")
		for _, f := range res.Conflicts {
			a.logf("  %s", f)
		}
		a.log("\nResolve them and commit, or run `git merge --abort` to back out.")
		return
	case "staged":
	default:
		a.log("\n[ERROR] git merge failed. Nothing was changed.")
		return
	}

	a.logf("\n✅ Merged %s into main — staged, NOT committed (%d file(s)):\n", branch, len(res.Staged))
	for i, f := range res.Staged {
		if i == 40 {
			a.logf("  ...and %d more", len(res.Staged)-40)
			break
		}
		a.logf("  %s", f)
	}
	a.log("\nPlease review the changes and make any necessary adjustments.")
	if !a.confirm("\nDo you want to commit the merge? (y/n): ") {
		a.log("\nMerge left staged in main. You can review and commit manually, or run `git merge --abort` to back out.")
		return
	}

	// Optionally fold "mark the plan Done" into the same merge commit: the plan file is already
	// staged by the merge, so bumping its status and re-staging lands the flip in the same commit.
	markedDone := false
	if task.Status != "Done" && a.confirm(fmt.Sprintf("\nAlso mark \"%s\" as Done in this commit? (y/n): ", task.Path)) {
		a.logf("\nMarking \"%s\" as Done...", task.Path)
		if a.setStatus(task.Path, "Done", false) {
			a.git("add", "--", agentDocsPath(task.Path))
			task.Status = "Done"
			markedDone = true
			a.log("Task marked as Done and staged for the merge commit.")
		} else {
			a.log("[WARN] Could not mark the task as Done. Committing the merge without the status change.")
		}
	}

	msg := fmt.Sprintf("Merge worktree branch \"%s\" for %s", branch, task.Path)
	if markedDone {
		msg += " (mark Done)"
	}
	a.logf("\nCommitting merge: \"%s\"...", msg)
	a.git("commit", "-m", msg)

	a.log("\nRemoving worktree and branch...")
	cleaned := worktree.RemoveWorktreeAndBranch(a.runner(), a.Root, wt.Path, branch)
	if cleaned.Reason != "" {
		a.logf("\n[WARN] %s", cleaned.Reason)
	}
	if cleaned.WorktreeRemoved && cleaned.BranchDeleted {
		a.log("✅ Worktree and branch successfully removed.")
	}

	a.log("\nRefreshing the plan index...")
	cli := planstatus.CLI{Root: a.Root, Stdout: a.Out, Stderr: a.Out}
	if cli.Run([]string{"regenerate"}) == 0 {
		index := filepath.Join(a.Root, "agent_docs", "plan_index.html")
		if _, err := os.Stat(index); err == nil {
			a.logf("Open %s to review the updated plans.", index)
		}
	}
}

// promptCleanup is a separate pass because --no-commit means the merge is not in history yet when
// the merge step finishes: `git branch -d` would rightly refuse, and removing the worktree before
// the user has committed would destroy their only copy of the work if they then aborted.
func (a App) promptCleanup(branch, worktreePath string) {
	if !a.confirm(fmt.Sprintf("\nDelete the worktree at %s? (y/n): ", worktreePath)) {
		a.logf("Left the worktree and branch \"%s\" in place.", branch)
		return
	}
	a.log("\nRemoving worktree...")
	cleaned := worktree.RemoveWorktreeAndBranch(a.runner(), a.Root, worktreePath, branch)
	if cleaned.Reason != "" {
		a.logf("\n[WARN] %s", cleaned.Reason)
	}
	if cleaned.WorktreeRemoved && cleaned.BranchDeleted {
		a.log("✅ Worktree and branch removed.")
	}
}
