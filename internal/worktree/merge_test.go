package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"work_dot_bat/internal/planstatus"
)

func TestMergeAvailabilityGate(t *testing.T) {
	live := &planstatus.Worktree{Path: "C:/r/p-x", Branch: "x"}
	stale := &planstatus.Worktree{Path: "C:/r/p-x", Branch: "x", Prunable: true}
	cases := []struct {
		name    string
		dirty   bool
		wt      *planstatus.Worktree
		current string
		want    Availability
	}{
		{"available", false, live, "main", Availability{Available: true}},
		{"dirty", true, live, "main", Availability{Reason: "commit or stash your changes first"}},
		{"no worktree", false, nil, "main", Availability{Reason: `no worktree for branch "x"`}},
		{"stale", false, stale, "main", Availability{Reason: "worktree for \"x\" is a stale registration — run `git worktree prune`"}},
		{"not on main", false, live, "dev", Availability{Reason: `main is not checked out (currently on "dev")`}},
		{"dirty reported first", true, nil, "dev", Availability{Reason: "commit or stash your changes first"}},
	}
	for _, c := range cases {
		if got := MergeAvailability(c.dirty, c.wt, "x", c.current); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
	if MergeMenuLabel(Availability{Available: true}) != "Merge back into main" {
		t.Error("available label")
	}
	if got := MergeMenuLabel(Availability{Reason: "why"}); got != "[Merge back into main — why]" {
		t.Error(got)
	}
}

func TestParseCommitList(t *testing.T) {
	got := ParseCommitList("abc123 Fix: the thing (#4)\n\n  def456   \nlonely\n")
	want := []Commit{{"abc123", "Fix: the thing (#4)"}, {"def456", ""}, {"lonely", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v", got)
	}
	if len(ParseCommitList("")) != 0 {
		t.Error("empty")
	}
}

func TestGetIncompleteTasksForBranch(t *testing.T) {
	all := []planstatus.Entry{
		{Path: "plans/x/plan.md", Status: "In Progress"},
		{Path: "plans/x/01.md", Status: "Ready for Review", SubtaskOf: "plans/x/plan.md"},
		{Path: "plans/x/02.md", Status: "Not Started", SubtaskOf: "plans/x/plan.md"},
		{Path: "plans/y.md", Status: "Not Started"},
	}
	branch := planstatus.BranchNameForPlan("plans/x/plan.md", all)
	got := GetIncompleteTasksForBranch(branch, all)
	if len(got) != 2 || got[0].Path != "plans/x/plan.md" || got[1].Path != "plans/x/02.md" {
		t.Errorf("%v", got)
	}
	if GetIncompleteTasksForBranch("", all) != nil {
		t.Error("empty branch")
	}
}

// mergeRepo: <base>/proj on main with shared.txt, plus a worktree <base>/proj-feat on branch
// feat holding one committed change. Returns the repo root and the worktree path.
func mergeRepo(t *testing.T) (root, wt string) {
	t.Helper()
	_, r := repoWithPlans(t)
	git(t, r, "config", "core.autocrlf", "false")
	os.WriteFile(filepath.Join(r, "shared.txt"), []byte("base\n"), 0o644)
	git(t, r, "add", ".")
	git(t, r, "commit", "-q", "-m", "shared")
	wt = filepath.ToSlash(filepath.Join(filepath.Dir(r), "proj-feat"))
	git(t, r, "worktree", "add", "-q", wt, "-b", "feat")
	return r, wt
}

func commitIn(t *testing.T, dir, file, content, msg string) {
	t.Helper()
	os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", msg)
}

func TestMergeStagesWithoutCommitting(t *testing.T) {
	root, wt := mergeRepo(t)
	commitIn(t, wt, "feature.txt", "new\n", "add feature")
	before := git(t, root, "rev-parse", "HEAD")

	commits := CommitsToMerge(nil, root, "feat")
	if len(commits) != 1 || commits[0].Subject != "add feature" {
		t.Fatalf("commits %v", commits)
	}
	if IsBranchMerged(nil, root, "feat") {
		t.Error("an unmerged branch reported merged")
	}
	res := MergeBranchIntoMain(nil, RunCommand, root, "feat")
	if res.Outcome != "staged" || !reflect.DeepEqual(res.Staged, []string{"feature.txt"}) {
		t.Fatalf("%+v", res)
	}
	if after := git(t, root, "rev-parse", "HEAD"); after != before {
		t.Error("the merge must not commit")
	}
	if got := git(t, root, "diff", "--cached", "--name-only"); got != "feature.txt" {
		t.Errorf("staged %q", got)
	}
	if !IsTreeDirty(nil, root) {
		t.Error("a staged merge leaves the tree dirty")
	}

	// After the user commits, the branch is merged and cleanup removes worktree and branch.
	git(t, root, "commit", "-q", "-m", "merge feat")
	if !IsBranchMerged(nil, root, "feat") {
		t.Error("committed merge should read as merged")
	}
	res2 := RemoveWorktreeAndBranch(RunCommand, root, wt, "feat")
	if !res2.WorktreeRemoved || !res2.BranchDeleted || res2.Reason != "" {
		t.Fatalf("%+v", res2)
	}
	if _, err := os.Stat(wt); err == nil {
		t.Error("worktree still on disk")
	}
	if got := git(t, root, "branch", "--list", "feat"); got != "" {
		t.Errorf("branch still exists: %q", got)
	}
}

func TestMergeConflictIsLeftInProgress(t *testing.T) {
	root, wt := mergeRepo(t)
	commitIn(t, wt, "shared.txt", "from feat\n", "feat edit")
	commitIn(t, root, "shared.txt", "from main\n", "main edit")

	res := MergeBranchIntoMain(nil, RunCommand, root, "feat")
	if res.Outcome != "conflict" || !reflect.DeepEqual(res.Conflicts, []string{"shared.txt"}) {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "MERGE_HEAD")); err != nil {
		t.Error("the merge must be left in progress, not aborted")
	}
	got, _ := os.ReadFile(filepath.Join(root, "shared.txt"))
	if !strings.Contains(string(got), "<<<<<<<") {
		t.Errorf("conflict markers missing:\n%s", got)
	}
}

func TestMergeFailureWithoutConflictsIsFailed(t *testing.T) {
	root, _ := mergeRepo(t)
	if res := MergeBranchIntoMain(nil, RunCommand, root, "no-such-branch"); res.Outcome != "failed" {
		t.Errorf("%+v", res)
	}
}

func TestCleanupReportsRefusalsWithoutForcing(t *testing.T) {
	root, wt := mergeRepo(t)
	commitIn(t, wt, "feature.txt", "new\n", "add feature")
	// A locked worktree makes `git worktree remove` refuse; the branch must be left alone.
	git(t, root, "worktree", "lock", wt)
	res := RemoveWorktreeAndBranch(RunCommand, root, wt, "feat")
	if res.WorktreeRemoved || res.BranchDeleted || !strings.Contains(res.Reason, "Could not remove") || !strings.Contains(res.Reason, wt) {
		t.Fatalf("%+v", res)
	}
	if got := git(t, root, "branch", "--list", "feat"); !strings.Contains(got, "feat") {
		t.Error("branch must survive a refused worktree removal")
	}
	git(t, root, "worktree", "unlock", wt)

	// Worktree removable but the branch is unmerged: `branch -d` refuses and is not forced.
	res = RemoveWorktreeAndBranch(RunCommand, root, wt, "feat")
	if !res.WorktreeRemoved || res.BranchDeleted || !strings.Contains(res.Reason, `git refused to delete branch "feat"`) {
		t.Fatalf("%+v", res)
	}
	if got := git(t, root, "branch", "--list", "feat"); !strings.Contains(got, "feat") {
		t.Error("an unmerged branch must not be force-deleted")
	}
}

func TestCleanupNeverForces(t *testing.T) {
	var calls []string
	run := func(cmd string, args []string, o RunOpts) (int, error) {
		calls = append(calls, strings.Join(args, " "))
		return 0, nil
	}
	RemoveWorktreeAndBranch(run, "/r", "/r-x", "x")
	if !reflect.DeepEqual(calls, []string{"worktree remove /r-x", "branch -d x"}) {
		t.Errorf("%v", calls)
	}
}

func TestCommitWorktree(t *testing.T) {
	root, wt := mergeRepo(t)
	_ = root
	if got := CommitWorktree(RunCommand, wt, "nothing"); got != "no-changes" {
		t.Errorf("clean tree: %q", got)
	}
	os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x"), 0o644)
	// Identity for the commit comes from the environment of this test process.
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@t")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@t")
	if got := CommitWorktree(RunCommand, wt, "plan x"); got != "committed" {
		t.Errorf("dirty tree: %q", got)
	}
	if got := git(t, wt, "log", "-1", "--format=%s"); got != "auto: plan x" {
		t.Errorf("message %q", got)
	}
	if got := CommitWorktree(RunCommand, wt, "auto: already prefixed"); got != "no-changes" {
		t.Errorf("%q", got)
	}

	var calls [][]string
	run := func(cmd string, args []string, o RunOpts) (int, error) {
		calls = append(calls, append([]string{o.Dir}, args...))
		return 0, nil
	}
	CommitWorktree(run, "/wt", "auto: msg")
	for _, c := range calls {
		if c[0] != "/wt" {
			t.Errorf("a worktree command ran without the worktree dir: %v", c)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip()
	}
}
