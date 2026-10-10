package planstatus

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func plan(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, "agent_docs", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "agent_docs", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func run(t *testing.T, root string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = CLI{Root: root, Stdout: &o, Stderr: &e}.Run(args)
	return code, o.String(), e.String()
}

func TestSetChangesOnlyStatusLine(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: Not Started\nauto: true\npriority: P2\n---\n\n# A\nbody\n")
	code, out, _ := run(t, root, "set", "plans/a.md", "In Progress")
	if code != 0 || out != "Set plans/a.md -> In Progress\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	want := "---\nstatus: In Progress\nauto: true\npriority: P2\n---\n\n# A\nbody\n"
	if got := read(t, root, "plans/a.md"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSetCancelledNeedsHuman(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: Not Started\n---\n\nx\n")
	code, _, errOut := run(t, root, "set", "plans/a.md", "Cancelled")
	if code != 1 || !strings.Contains(errOut, `Refusing to set restricted status "Cancelled".`) {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if code, _, _ := run(t, root, "set", "plans/a.md", "Cancelled", "--human"); code != 0 {
		t.Fatalf("--human refused, code=%d", code)
	}
	if !strings.HasPrefix(read(t, root, "plans/a.md"), "---\nstatus: Cancelled\n") {
		t.Fatal("status not written")
	}
}

func TestSetRejectsInvalidAndMissing(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: Not Started\n---\n\nx\n")
	if _, _, e := run(t, root, "set", "plans/a.md", "Bogus"); !strings.Contains(e, `Invalid status "Bogus". Allowed: Not Started, In Progress,`) {
		t.Fatalf("err=%q", e)
	}
	if code, _, e := run(t, root, "set", "plans/none.md", "Done"); code != 1 || e != "No entry found for \"plans/none.md\"\n" {
		t.Fatalf("code=%d err=%q", code, e)
	}
}

func TestDoneRefusedWithOpenSubtask(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/x/plan.md", "---\nstatus: In Progress\n---\n\nparent\n")
	plan(t, root, "plans/x/01.md", "---\nstatus: Not Started\nsubtask_of: plans/x/plan.md\n---\n\nchild\n")
	code, _, e := run(t, root, "set", "plans/x/plan.md", "Done")
	if code != 1 || !strings.Contains(e, "  ! plans/x/01.md (Not Started)") {
		t.Fatalf("code=%d err=%q", code, e)
	}
	run(t, root, "set", "plans/x/01.md", "Done")
	if code, _, e := run(t, root, "set", "plans/x/plan.md", "Done"); code != 0 {
		t.Fatalf("code=%d err=%q", code, e)
	}
}

func TestReadyForReviewManualNeedsSteps(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: In Progress\nverification: manual\n---\n\n## Manual Verification\n\n## Next\n")
	if code, _, e := run(t, root, "set", "plans/a.md", "Ready for Review"); code != 1 || !strings.Contains(e, "has no\n") {
		t.Fatalf("code=%d err=%q", code, e)
	}
	plan(t, root, "plans/a.md", "---\nstatus: In Progress\nverification: manual\n---\n\n## Manual Verification\n1. click it\n")
	if code, _, _ := run(t, root, "set", "plans/a.md", "Ready for Review"); code != 0 {
		t.Fatal("refused despite steps")
	}
}

func TestAddInFolderWithPlanMdBecomesSubtask(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/x/plan.md", "---\nstatus: Not Started\n---\n\np\n")
	plan(t, root, "plans/x/01.md", "# one\n")
	code, out, _ := run(t, root, "add", "plans/x/01.md")
	if code != 0 || out != "Added plans/x/01.md (Not Started)\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if got := read(t, root, "plans/x/01.md"); got != "---\nstatus: Not Started\nsubtask_of: plans/x/plan.md\n---\n\n# one\n" {
		t.Fatalf("got %q", got)
	}
	if code, _, e := run(t, root, "add", "plans/x/01.md"); code != 1 || e != "Entry already exists for \"plans/x/01.md\"\n" {
		t.Fatalf("code=%d err=%q", code, e)
	}
}

func TestAddSubtaskOutput(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/p.md", "---\nstatus: Not Started\n---\n\np\n")
	plan(t, root, "plans/c.md", "# c\n")
	code, out, _ := run(t, root, "add-subtask", "agent_docs/plans/c.md", "-parent", "plans/p.md")
	want := "Added subtask plans/c.md (subtask of plans/p.md) (Not Started)\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if !strings.Contains(read(t, root, "plans/c.md"), "subtask_of: plans/p.md") {
		t.Fatal("subtask_of missing")
	}
}

func TestSuggestionsDeriveAndFilter(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/agent_suggestions/s.md", "---\nstatus: Not Started\n---\n\ns\n")
	plan(t, root, "plans/agent_suggestions/auto.md", "---\nstatus: Not Started\nauto: true\n---\n\ns\n")
	plan(t, root, "plans/done/old.md", "---\nstatus: Done\n---\n\no\n")
	_, out, _ := run(t, root, "list")
	if out != "Not Started\tplans/agent_suggestions/auto.md\nSuggestion\tplans/agent_suggestions/s.md\n" {
		t.Fatalf("out=%q", out)
	}
	_, out, _ = run(t, root, "list", "Suggestion")
	if out != "Suggestion\tplans/agent_suggestions/s.md\n" {
		t.Fatalf("out=%q", out)
	}
}

func TestBackfillAndEnsure(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/x/plan.md", "# parent\n")
	plan(t, root, "plans/x/01.md", "---\nstatus: In Progress\n---\n\nc\n")
	_, out, _ := run(t, root, "ensure-frontmatter")
	if out != "Added frontmatter to 1 plan file(s).\n  + plans/x/plan.md\n" {
		t.Fatalf("out=%q", out)
	}
	_, out, _ = run(t, root, "backfill-subtasks")
	if out != "Backfilled subtask_of for 1 plan file(s).\n  + plans/x/01.md\n" {
		t.Fatalf("out=%q", out)
	}
	if got := read(t, root, "plans/x/01.md"); !strings.HasPrefix(got, "---\nstatus: In Progress\nsubtask_of: plans/x/plan.md\n---\n") {
		t.Fatalf("got %q", got)
	}
}

func TestBlockedByAndSetVerification(t *testing.T) {
	fm := ParseFrontmatter("---\nstatus: Not Started\nblocked_by:\n  - agent_docs/plans/a.md\n  - plans/b.md\npriority: p3\n---\nbody")
	if len(fm.BlockedBy) != 2 || fm.BlockedBy[0] != "plans/a.md" || fm.Priority != 3 || fm.Body != "body" {
		t.Fatalf("%+v", fm)
	}
	fm = ParseFrontmatter("---\nstatus: X\nblocked_by: [plans/a.md, plans/b.md]\n---\n")
	if len(fm.BlockedBy) != 2 {
		t.Fatalf("%+v", fm)
	}
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: Not Started\n---\n\nx\n")
	run(t, root, "set-verification", "plans/a.md", "manual")
	if got := read(t, root, "plans/a.md"); got != "---\nstatus: Not Started\nverification: manual\n---\n\nx\n" {
		t.Fatalf("got %q", got)
	}
	run(t, root, "set-verification", "plans/a.md", "automated")
	if got := read(t, root, "plans/a.md"); got != "---\nstatus: Not Started\n---\n\nx\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCheckReportsMissingFrontmatter(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/ok.md", "---\nstatus: Not Started\n---\n\nx\n")
	plan(t, root, "plans/bare.md", "no frontmatter\n")
	list := func(string) ([]string, error) { return []string{"plans/bare.md", "plans/ok.md"}, nil }
	r, err := Check(root, CheckOptions{ListPlanFiles: list})
	if err != nil || r.OK || len(r.MissingFrontmatter) != 1 || r.MissingFrontmatter[0] != "plans/bare.md" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	r, _ = Check(root, CheckOptions{ListPlanFiles: func(string) ([]string, error) { return []string{"plans/ok.md"}, nil }})
	if !r.OK {
		t.Fatalf("clean tree not ok: %+v", r)
	}
}

func TestCheckDoneParentOpenSubtaskAndStale(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/x/plan.md", "---\nstatus: Done\n---\n\np\n")
	plan(t, root, "plans/x/01.md", "---\nstatus: Blocked - User Input Required\nsubtask_of: plans/x/plan.md\n---\n\nc\n")
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	r, _ := Check(root, CheckOptions{
		ListPlanFiles:         func(string) ([]string, error) { return []string{"plans/x/01.md", "plans/x/plan.md"}, nil },
		Now:                   now,
		FileCommitTime:        func(string, string) (time.Time, bool) { return now.Add(-20 * 24 * time.Hour), true },
		HasWorkingTreeChanges: func(string, string) bool { return false },
	})
	if r.OK || len(r.OpenSubtasksForDoneParents) != 1 || len(r.StaleBlockedPlans) != 1 || r.StaleBlockedPlans[0].Days != 20 {
		t.Fatalf("%+v", r)
	}
}

func TestWorktreeOverlayAndDivergence(t *testing.T) {
	entries := []Entry{{Path: "plans/a.md", Status: "Not Started"}, {Path: "plans/b.md", Status: "Done"}}
	wt := map[string]WorktreeStatus{
		"plans/a.md": {Status: "Ready for Review", Branch: "feat", Worktree: "/w"},
		"plans/b.md": {Status: "In Progress", Branch: "feat"},
		"plans/c.md": {Status: "In Progress", Branch: "feat"},
	}
	got := ApplyWorktreeStatuses(entries, wt)
	if got[0].Status != "Ready for Review" || got[0].WorktreeBranch != "feat" || got[1].Status != "Done" || got[2].Path != "plans/c.md" {
		t.Fatalf("%+v", got)
	}
	done := "Done"
	div := ClassifyWorktreeDivergence([]FileStatus{{Path: "plans/b.md", Status: &done}},
		map[string]WorktreeStatus{"plans/b.md": {Status: "In Progress", Branch: "feat", BranchTouchedPlan: true, CommitsBehind: 3}})
	if len(div) != 1 || div[0].CommitsBehind != 3 {
		t.Fatalf("%+v", div)
	}
}

func TestShimOnlyWithDashboard(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/a.md", "---\nstatus: Not Started\n---\n\nx\n")
	if _, out, _ := run(t, root, "regenerate"); out != "No agent_docs/plan_index.html in this repo; nothing to regenerate.\n" {
		t.Fatalf("out=%q", out)
	}
	os.WriteFile(filepath.Join(root, "agent_docs", "plan_index.html"), []byte("<html>"), 0o644)
	run(t, root, "regenerate")
	want := "// GENERATED from plan file frontmatter by scripts/work_dot_bat/plan-status.sh — do not edit by hand.\n" +
		"window.planData = [\n  {\n    \"path\": \"plans/a.md\",\n    \"status\": \"Not Started\"\n  }\n];\n"
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs", "plan_status.js"))
	if string(b) != want {
		t.Fatalf("got %q", b)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, e := run(t, t.TempDir(), "frobnicate")
	if code != 1 || !strings.HasPrefix(e, "Unknown command: \"frobnicate\"\nUsage: plan-status.sh <add|") {
		t.Fatalf("code=%d err=%q", code, e)
	}
}
