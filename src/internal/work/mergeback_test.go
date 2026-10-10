package work

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/internal/planstatus"
	"work_dot_bat/internal/taskmenu"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// mergeFixture: <base>/proj on main with plan x.md (In Progress) and a worktree on the plan's
// branch where x.md is Ready for Review plus one new file, all committed.
func mergeFixture(t *testing.T, inputs ...string) (*fixture, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	f := newFixture(t, inputs...)
	base := t.TempDir()
	root := filepath.ToSlash(filepath.Join(base, "proj"))
	os.MkdirAll(root, 0o755)
	f.root, f.app.Root = root, root
	writePlan(t, root, "plans/x.md", "---\nstatus: In Progress\n---\n\n# GOAL\nDo x\n")
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "config", "core.autocrlf", "false")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "init")

	branch := planstatus.BranchNameForPlan("plans/x.md", planstatus.ReadStatus(root))
	wt := filepath.ToSlash(filepath.Join(base, "proj-"+branch))
	gitIn(t, root, "worktree", "add", "-q", wt, "-b", branch)
	writePlan(t, wt, "plans/x.md", "---\nstatus: Ready for Review\n---\n\n# GOAL\nDo x\n")
	os.WriteFile(filepath.Join(wt, "feature.txt"), []byte("feature\n"), 0o644)
	gitIn(t, wt, "add", ".")
	gitIn(t, wt, "commit", "-q", "-m", "implement x")

	f.app.Worktrees = nil // the real `git worktree list`
	f.app.Git = nil       // real git
	f.app.Tasks = func() []planstatus.Entry {
		entries := planstatus.ReadStatus(root)
		for i := range entries {
			if entries[i].Path == "plans/x.md" {
				entries[i].Status = "Ready for Review"
			}
		}
		return entries
	}
	return f, branch, wt
}

func readyTask() *taskmenu.Item {
	return &taskmenu.Item{Entry: planstatus.Entry{Path: "plans/x.md", Status: "Ready for Review"}}
}

func TestMergeBackHappyPathCommitsMarksDoneAndCleansUp(t *testing.T) {
	f, branch, wt := mergeFixture(t, "y", "y")
	task := readyTask()
	f.app.mergeBackWorkflow(task)
	out := f.out.String()

	wantMsg := `Merge worktree branch "` + branch + `" for plans/x.md (mark Done)`
	if got := gitIn(t, f.root, "log", "-1", "--format=%s"); got != wantMsg {
		t.Errorf("merge commit %q\n%s", got, out)
	}
	if len(strings.Fields(gitIn(t, f.root, "log", "--merges", "--format=%h"))) != 1 {
		t.Error("expected exactly one merge commit (--no-ff)")
	}
	if got := planStatusOf(t, f.root, "plans/x.md"); got != "Done" {
		t.Errorf("status on main %q", got)
	}
	if gitIn(t, f.root, "status", "--porcelain") != "" {
		t.Error("the tree should be clean after the committed merge")
	}
	if _, err := os.Stat(wt); err == nil {
		t.Error("worktree not removed")
	}
	if gitIn(t, f.root, "branch", "--list", branch) != "" {
		t.Error("branch not deleted")
	}
	for _, want := range []string{
		"Commits to be merged (main.." + branch + "):", "implement x", "Merging (--no-ff --no-commit)...",
		"staged, NOT committed (2 file(s))", "Do you want to commit the merge? (y/n): ",
		`Also mark "plans/x.md" as Done in this commit? (y/n): `, "✅ Worktree and branch successfully removed.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestMergeBackLeavesMergeStagedWhenNotCommitted(t *testing.T) {
	f, branch, wt := mergeFixture(t, "n")
	before := gitIn(t, f.root, "rev-parse", "HEAD")
	f.app.mergeBackWorkflow(readyTask())
	if gitIn(t, f.root, "rev-parse", "HEAD") != before {
		t.Error("nothing may be committed without consent")
	}
	if !strings.Contains(gitIn(t, f.root, "diff", "--cached", "--name-only"), "feature.txt") {
		t.Error("the merge should be staged")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Error("the worktree must stay until the merge is committed")
	}
	if gitIn(t, f.root, "branch", "--list", branch) == "" {
		t.Error("the branch must stay")
	}
	if !strings.Contains(f.out.String(), "Merge left staged in main.") {
		t.Errorf("%s", f.out.String())
	}
}

func TestMergeBackReportsConflictAndLeavesItInProgress(t *testing.T) {
	f, _, _ := mergeFixture(t)
	// main edits the same plan the branch edited.
	writePlan(t, f.root, "plans/x.md", "---\nstatus: Blocked - User Input Required\n---\n\n# GOAL\nDo x differently\n")
	gitIn(t, f.root, "commit", "-q", "-am", "main edit")
	f.app.mergeBackWorkflow(readyTask())
	out := f.out.String()
	if !strings.Contains(out, "[CONFLICT] The merge is left in progress. Conflicting paths:") || !strings.Contains(out, "  agent_docs/plans/x.md") ||
		!strings.Contains(out, "Resolve them and commit, or run `git merge --abort` to back out.") {
		t.Errorf("%s", out)
	}
	if _, err := os.Stat(filepath.Join(f.root, ".git", "MERGE_HEAD")); err != nil {
		t.Error("the conflicted merge must be left in progress")
	}
}

func TestMergeBackGateMessagesAndWarnings(t *testing.T) {
	// Dirty main checkout disables the flow with the gate's reason.
	f, _, _ := mergeFixture(t)
	os.WriteFile(filepath.Join(f.root, "dirty.txt"), []byte("x"), 0o644)
	f.app.mergeBackWorkflow(readyTask())
	if !strings.Contains(f.out.String(), "\nCannot merge back: commit or stash your changes first") {
		t.Errorf("%s", f.out.String())
	}

	// No worktree for the plan's branch.
	g, _, _ := mergeFixture(t)
	other := &taskmenu.Item{Entry: planstatus.Entry{Path: "plans/other.md", Status: "Done"}}
	g.app.mergeBackWorkflow(other)
	if !strings.Contains(g.out.String(), "Cannot merge back: no worktree for branch ") {
		t.Errorf("%s", g.out.String())
	}

	// A sibling plan on the shared branch is still incomplete: warn, and decline to cancel.
	h, branch, _ := mergeFixture(t, "n")
	h.app.Tasks = func() []planstatus.Entry {
		return []planstatus.Entry{
			{Path: "plans/x.md", Status: "Ready for Review"},
			{Path: "plans/x/01.md", Status: "Not Started", SubtaskOf: "plans/x.md"},
		}
	}
	before := gitIn(t, h.root, "rev-parse", "HEAD")
	h.app.mergeBackWorkflow(readyTask())
	out := h.out.String()
	if !strings.Contains(out, `[WARNING] The following plan(s) sharing branch "`+branch+`" are still incomplete:`) ||
		!strings.Contains(out, "  - [Not Started] plans/x/01.md") || !strings.Contains(out, "Merge cancelled.") {
		t.Errorf("%s", out)
	}
	if gitIn(t, h.root, "rev-parse", "HEAD") != before || gitIn(t, h.root, "status", "--porcelain") != "" {
		t.Error("declining the warning must change nothing")
	}
}

func TestMergeBackAlreadyMergedOffersCleanup(t *testing.T) {
	f, branch, wt := mergeFixture(t, "y")
	gitIn(t, f.root, "merge", "-q", "--no-ff", "-m", "merged by hand", branch)
	f.app.mergeBackWorkflow(readyTask())
	if !strings.Contains(f.out.String(), `"`+branch+`" is already merged into main.`) ||
		!strings.Contains(f.out.String(), "Delete the worktree at "+wt+"? (y/n): ") {
		t.Errorf("%s", f.out.String())
	}
	if _, err := os.Stat(wt); err == nil {
		t.Error("worktree should be removed after consent")
	}

	// Declining leaves both in place.
	g, branch, wt := mergeFixture(t, "n")
	gitIn(t, g.root, "merge", "-q", "--no-ff", "-m", "merged by hand", branch)
	g.app.mergeBackWorkflow(readyTask())
	if _, err := os.Stat(wt); err != nil || !strings.Contains(g.out.String(), `Left the worktree and branch "`+branch+`" in place.`) {
		t.Errorf("%s", g.out.String())
	}
}
