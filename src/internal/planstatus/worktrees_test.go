package planstatus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugAndBranchNames(t *testing.T) {
	cases := map[string]string{
		"plans/august_2026/Foo Bar/plan.md": "foo-bar",
		"plans/x/01_Go Scaffold.md":         "01_go-scaffold",
		`plans\a\plan.md`:                   "a",
	}
	for in, want := range cases {
		if got := SlugForPlan(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	entries := []Entry{
		{Path: "plans/x/plan.md", Status: "In Progress"},
		{Path: "plans/x/01.md", Status: "Not Started", SubtaskOf: "plans/x/plan.md"},
		{Path: "plans/x/02.md", Status: "Not Started", SubtaskOf: "plans/x/plan.md", Auto: true},
	}
	if got := BranchNameForPlan("agent_docs/plans/x/01.md", entries); got != "x" {
		t.Errorf("subtask branch = %q", got)
	}
	if got := BranchNameForPlan("plans/x/02.md", entries); got != "02" {
		t.Errorf("auto subtask of a non-auto parent keeps its own branch, got %q", got)
	}
	if got := BranchNameForPlan("plans/unknown.md", entries); got != "unknown" {
		t.Errorf("unregistered plan = %q", got)
	}
}

func TestParseWorktreeList(t *testing.T) {
	wts := ParseWorktreeList("worktree C:\\repo\nHEAD abc\nbranch refs/heads/main\n\nworktree C:/repo-wt\nbranch refs/heads/feat\nprunable gone\n")
	if len(wts) != 2 || wts[0].Path != "C:/repo" || wts[0].Branch != "main" || wts[1].Branch != "feat" || !wts[1].Prunable {
		t.Fatalf("%+v", wts)
	}
}

func TestFossilDetection(t *testing.T) {
	d := BranchDivergence{CommitsBehind: 3, MainChangedPlans: map[string]bool{"p": true}, BranchChangedPlans: map[string]bool{}}
	if !IsFossilPlanStatus("p", d) {
		t.Error("main moved it, branch did not: fossil")
	}
	d.BranchChangedPlans["p"] = true
	if IsFossilPlanStatus("p", d) {
		t.Error("both touched, close behind: not a fossil")
	}
	d.CommitsBehind = FossilBranchCommitThreshold + 1
	if !IsFossilPlanStatus("p", d) {
		t.Error("both touched, far behind: fossil")
	}
}

func TestCollectWorktreeStatusesWithFakeGit(t *testing.T) {
	main, wt := t.TempDir(), t.TempDir()
	plan(t, wt, "plans/a.md", "---\nstatus: Ready for Review\n---\n\nx\n")
	git := func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "worktree":
			return "worktree " + filepath.ToSlash(main) + "\nbranch refs/heads/main\n\nworktree " + filepath.ToSlash(wt) + "\nbranch refs/heads/feat\n", nil
		case "rev-list":
			return "2\n", nil
		case "diff":
			if strings.Contains(args[2], "...feat") {
				return "agent_docs/plans/a.md\n", nil
			}
			return "", nil
		}
		return "", os.ErrInvalid
	}
	got, err := CollectWorktreeStatusesWith(main, git)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := got["plans/a.md"]
	if !ok || a.Status != "Ready for Review" || a.Branch != "feat" || a.Fossil || !a.BranchTouchedPlan || a.CommitsBehind != 2 {
		t.Fatalf("%+v", got)
	}
}
