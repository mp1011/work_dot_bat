package taskmenu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/planstatus"
)

func plan(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, "agent_docs", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fm(status, extra string) string {
	return "---\nstatus: " + status + "\n" + extra + "---\n\nbody\n"
}

// fixture: a parent with two prioritized subtasks (listed out of order) and one unranked,
// a standalone plan, a done plan, a cancelled plan and two parked suggestions.
func fixture(t *testing.T) string {
	root := t.TempDir()
	plan(t, root, "plans/a_parent/plan.md", fm("In Progress", ""))
	plan(t, root, "plans/a_parent/01_low.md", fm("Not Started", "subtask_of: plans/a_parent/plan.md\npriority: P3\n"))
	plan(t, root, "plans/a_parent/02_high.md", fm("Not Started", "subtask_of: plans/a_parent/plan.md\npriority: P1\n"))
	plan(t, root, "plans/a_parent/03_none.md", fm("Not Started", "subtask_of: plans/a_parent/plan.md\n"))
	plan(t, root, "plans/b_solo.md", fm("Not Started", "auto: true\n"))
	plan(t, root, "plans/c_done.md", fm("Done", ""))
	plan(t, root, "plans/d_cancelled.md", fm("Cancelled", ""))
	plan(t, root, "plans/agent_suggestions/s_one.md", "---\nstatus: Not Started\n---\n\n# GOAL\n\n# Tidy the widgets\n")
	plan(t, root, "plans/agent_suggestions/s_two.md", fm("Not Started", ""))
	return root
}

type harness struct {
	menu Menu
	out  []string
	left []string
}

func newHarness(t *testing.T, root string, inputs ...string) *harness {
	h := &harness{left: inputs}
	h.menu = Menu{
		IO: menu.IO{
			Ask: func(prompt string) (string, bool) {
				h.out = append(h.out, "PROMPT: "+prompt)
				if len(h.left) == 0 {
					return "", false
				}
				in := h.left[0]
				h.left = h.left[1:]
				return in, true
			},
			Log: func(msg string) { h.out = append(h.out, msg) },
		},
		Root:              root,
		Tasks:             func() []planstatus.Entry { return planstatus.ReadStatus(root) },
		EnsureFrontmatter: func() []string { return nil },
		AddPlan:           func(p string) error { return planstatus.Add(root, p, "Not Started") },
	}
	return h
}

func (h *harness) text() string { return strings.Join(h.out, "\n") }

func TestMenuRendersGoldenList(t *testing.T) {
	h := newHarness(t, fixture(t), "0")
	if r := h.menu.Select(); r.Task != nil || r.Quit {
		t.Fatalf("skip should select nothing: %+v", r)
	}
	want := strings.Join([]string{
		"Incomplete Tasks:\n",
		"  1) [In Progress] plans/a_parent/plan.md",
		"     ↳ 2) [Not Started] plans/a_parent/02_high.md [P1]",
		"     ↳ 3) [Not Started] plans/a_parent/01_low.md [P3]",
		"     ↳ 4) [Not Started] plans/a_parent/03_none.md",
		"  5) [Not Started] plans/b_solo.md [auto]",
		"  w) [Write-in task]",
		"  p) [Paste path to a new plan file]",
		"  v) [View 2 parked suggestions]",
		"  0) [Skip / No task selected]",
		"",
		"PROMPT: Select a task (1-5), 'w' for write-in task, 'p' to paste path, 'v' to view 2 parked suggestion(s), '0' / 's' to skip, or 'q' to quit: ",
	}, "\n")
	if got := h.text(); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSelectByNumberAndByPath(t *testing.T) {
	root := fixture(t)
	if r := newHarness(t, root, "2").menu.Select(); r.Task == nil || r.Task.Path != "plans/a_parent/02_high.md" || !r.Task.IsSubtask {
		t.Fatalf("number: %+v", r.Task)
	}
	if r := newHarness(t, root, "b_solo.md").menu.Select(); r.Task == nil || r.Task.Path != "plans/b_solo.md" {
		t.Fatalf("path: %+v", r.Task)
	}
}

func TestQuitAndSkipTouchNothing(t *testing.T) {
	root := fixture(t)
	before, _ := os.ReadFile(filepath.Join(root, "agent_docs/plans/b_solo.md"))
	h := newHarness(t, root, "q")
	if r := h.menu.Select(); !r.Quit || r.Task != nil {
		t.Fatalf("q: %+v", r)
	}
	if !strings.HasSuffix(h.text(), "Exiting work script.") {
		t.Fatalf("missing exit message:\n%s", h.text())
	}
	if r := newHarness(t, root, "").menu.Select(); r.Task != nil || r.Quit {
		t.Fatalf("blank: %+v", r)
	}
	after, _ := os.ReadFile(filepath.Join(root, "agent_docs/plans/b_solo.md"))
	if string(before) != string(after) {
		t.Fatal("plan file changed")
	}
}

func TestParkedSuggestionsViewAndBack(t *testing.T) {
	root := fixture(t)
	h := newHarness(t, root, "v", "b", "v", "1")
	if r := h.menu.Select(); r.Task == nil || r.Task.Path != "plans/agent_suggestions/s_one.md" {
		t.Fatalf("got %+v", r.Task)
	}
	for _, want := range []string{
		"2 parked suggestion(s) — agent_docs/plans/agent_suggestions/:\n",
		"   1. s_one — Tidy the widgets",
		"   2. s_two — body",
		"PROMPT: Select a suggestion (1-2), 'b' to go back, or 'q' to quit: ",
	} {
		if !strings.Contains(h.text(), want) {
			t.Errorf("missing %q in:\n%s", want, h.text())
		}
	}
	if n := strings.Count(h.text(), "Incomplete Tasks:"); n != 2 {
		t.Errorf("list should reprint after back, printed %d times", n)
	}
}

func TestWriteInCreatesRegisteredPlan(t *testing.T) {
	root := fixture(t)
	h := newHarness(t, root, "w", "a_parent", "extra_thing", "Do the thing")
	r := h.menu.Select()
	if r.Task == nil || r.Task.Path != "plans/a_parent/extra_thing.md" || r.Task.Status != "Not Started" {
		t.Fatalf("got %+v", r.Task)
	}
	b, err := os.ReadFile(filepath.Join(root, "agent_docs/plans/a_parent/extra_thing.md"))
	if err != nil {
		t.Fatal(err)
	}
	f := planstatus.ParseFrontmatter(string(b))
	if f.Status == nil || *f.Status != "Not Started" || f.SubtaskOf != "plans/a_parent/plan.md" || !strings.Contains(f.Body, "# GOAL\nDo the thing") {
		t.Fatalf("file:\n%s", b)
	}
	if !strings.Contains(h.text(), "Created plan file at: agent_docs/plans/a_parent/extra_thing.md") {
		t.Fatalf("output:\n%s", h.text())
	}
}

func TestWriteInRefusesToOverwrite(t *testing.T) {
	root := fixture(t)
	h := newHarness(t, root, "w", "plans", "b_solo", "clobber", "0")
	if r := h.menu.Select(); r.Task != nil {
		t.Fatalf("got %+v", r.Task)
	}
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs/plans/b_solo.md"))
	if strings.Contains(string(b), "clobber") {
		t.Fatal("existing plan overwritten")
	}
	if !strings.Contains(h.text(), "[ERROR] Plan file already exists: agent_docs/plans/b_solo.md") {
		t.Fatalf("output:\n%s", h.text())
	}
}

func TestPastePath(t *testing.T) {
	root := fixture(t)
	outside := filepath.Join(t.TempDir(), "x.md")
	os.WriteFile(outside, []byte("x"), 0o644)
	h := newHarness(t, root, "p", outside, "0")
	if r := h.menu.Select(); r.Task != nil {
		t.Fatalf("outside path selected: %+v", r.Task)
	}
	if !strings.Contains(h.text(), "[ERROR] Invalid path. Plan file must be inside agent_docs/: \""+outside+"\"") {
		t.Fatalf("output:\n%s", h.text())
	}

	// An untracked file inside agent_docs gets registered.
	plan(t, root, "plans/new_one.md", "# GOAL\nhi\n")
	h = newHarness(t, root, "p", "agent_docs\\plans\\new_one.md")
	if r := h.menu.Select(); r.Task == nil || r.Task.Path != "plans/new_one.md" {
		t.Fatalf("got %+v\n%s", r.Task, h.text())
	}
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs/plans/new_one.md"))
	if !strings.HasPrefix(string(b), "---\nstatus: Not Started\n") {
		t.Fatalf("not registered:\n%s", b)
	}

	// An already-tracked one is returned as is.
	h = newHarness(t, root, "p", "plans/b_solo.md")
	if r := h.menu.Select(); r.Task == nil || !strings.Contains(h.text(), `Plan "plans/b_solo.md" is already in the plan index.`) {
		t.Fatalf("output:\n%s", h.text())
	}
}

func TestInvalidSelectionMessage(t *testing.T) {
	h := newHarness(t, fixture(t), "99", "0")
	h.menu.Select()
	want := `Invalid selection "99". Please enter a number between 1 and 5, 'w' for write-in task, 'p' to paste a plan path, '0' to skip, or 'q' to quit.`
	if !strings.Contains(h.text(), want) {
		t.Fatalf("output:\n%s", h.text())
	}
}

func TestEmptyBacklogMenu(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/done.md", fm("Done", ""))
	h := newHarness(t, root, "0")
	h.menu.Select()
	want := "No incomplete tasks found in Task Index.\n\n  w) [Write-in task]\n  p) [Paste path to a new plan file]\n  0) [Skip / No task selected]\n\n" +
		"PROMPT: Select option ('w' for write-in task, 'p' to paste path, '0' / 's' to skip, or 'q' to quit): "
	if got := h.text(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestDependsOnHidesUnresolved(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/first.md", fm("In Progress", ""))
	plan(t, root, "plans/second.md", fm("Not Started", "depends_on: plans/first.md\n"))
	got := RunnableAndGrouped(planstatus.ReadStatus(root), root)
	if len(got) != 1 || got[0].Path != "plans/first.md" {
		t.Fatalf("got %+v", got)
	}
}

func TestDoneOnBranchStaysListedUntilMergedToMain(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/x.md", fm("Not Started", ""))
	tasks := []planstatus.Entry{{Path: "plans/x.md", Status: "Done", WorktreeBranch: "x"}}
	if got := RunnableAndGrouped(tasks, root); len(got) != 1 {
		t.Fatalf("branch-done should be listed: %+v", got)
	}
	tasks = append(tasks, planstatus.Entry{Path: "plans/x.md", Status: "Done"})
	if got := RunnableAndGrouped(tasks, root); len(got) != 0 {
		t.Fatalf("main-done should be hidden: %+v", got)
	}
}

func TestFormatTaskLineColorOnlyOnTTY(t *testing.T) {
	it := Item{Entry: planstatus.Entry{Path: "plans/x.md", Status: "In Progress", WorktreeBranch: "br"}}
	if got := FormatTaskLine(it, 0, nil, false); got != "  1) [In Progress @ br] plans/x.md" {
		t.Fatal(got)
	}
	if got := FormatTaskLine(it, 0, nil, true); got != "  1) [\x1b[33mIn Progress\x1b[0m @ br] plans/x.md" {
		t.Fatal(got)
	}
}

func TestResolvePlanPath(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/foo/bar.md", "x")
	cases := map[string]string{
		`"agent_docs\plans\foo\bar.md"`: "plans/foo/bar.md",
		"AGENT_DOCS/plans/foo/bar.md":   "plans/foo/bar.md",
		"foo/bar.md":                    "plans/foo/bar.md",
		"plans/new.md":                  "plans/new.md",
		filepath.Join(root, "agent_docs", "plans", "foo", "bar.md"): "plans/foo/bar.md",
		"../escape.md":              "",
		"agent_docs/plans/../../x":  "",
		filepath.Join(root, "x.md"): "",
		"   ":                       "",
	}
	for in, want := range cases {
		if got := ResolvePlanPath(in, root); got != want {
			t.Errorf("ResolvePlanPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanFoldersSkipArchives(t *testing.T) {
	root := fixture(t)
	plan(t, root, "plans/old_plans/x.md", "x")
	plan(t, root, "plans/done/y.md", "y")
	got := strings.Join(PlanFolders(root), ",")
	if got != "plans,plans/a_parent,plans/agent_suggestions" {
		t.Fatal(got)
	}
}

func TestParseIntMatchesJS(t *testing.T) {
	for in, want := range map[string]int{"2": 2, "2abc": 2, " 7": 7, "-1": -1} {
		if n, ok := parseInt(in); !ok || n != want {
			t.Errorf("%q -> %d,%v", in, n, ok)
		}
	}
	if _, ok := parseInt("abc"); ok {
		t.Error("abc parsed")
	}
}

func TestNestedPlansDisplayAndIndentation(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/parent/plan.md", fm("Not Started", ""))
	plan(t, root, "plans/parent/child/plan.md", fm("Not Started", "subtask_of: plans/parent/plan.md\n"))
	plan(t, root, "plans/parent/child/grandchild/plan.md", fm("Not Started", "subtask_of: plans/parent/child/plan.md\n"))

	h := newHarness(t, root, "0")
	h.menu.Select()

	wantLines := []string{
		"Incomplete Tasks:\n",
		"  1) [Not Started] plans/parent",
		"     ↳ 2) [Not Started] plans/parent/child",
		"        ↳ 3) [Not Started] plans/parent/child/grandchild",
	}
	for _, want := range wantLines {
		if !strings.Contains(h.text(), want) {
			t.Fatalf("expected output to contain %q, but got:\n%s", want, h.text())
		}
	}
}

func TestNestedPlansMultipleMDFilesNotOmitted(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/parent/plan.md", fm("Not Started", ""))
	plan(t, root, "plans/parent/extra.md", fm("Not Started", "subtask_of: plans/parent/plan.md\n"))
	plan(t, root, "plans/parent/child/plan.md", fm("Not Started", "subtask_of: plans/parent/plan.md\n"))
	plan(t, root, "plans/parent/child/grandchild/plan.md", fm("Not Started", "subtask_of: plans/parent/child/plan.md\n"))

	h := newHarness(t, root, "0")
	h.menu.Select()

	// In plans/parent, there are 2 md files (plan.md and extra.md), so plan.md is NOT omitted.
	// In plans/parent/child and grandchild, there is only one md file, so plan.md IS omitted.
	if !strings.Contains(h.text(), "  1) [Not Started] plans/parent/plan.md") {
		t.Fatalf("plans/parent/plan.md should not be omitted when multiple md files exist:\n%s", h.text())
	}
	if !strings.Contains(h.text(), "     ↳ 2) [Not Started] plans/parent/child") {
		t.Fatalf("plans/parent/child should be omitted:\n%s", h.text())
	}
	if !strings.Contains(h.text(), "        ↳ 3) [Not Started] plans/parent/child/grandchild") {
		t.Fatalf("plans/parent/child/grandchild should be omitted:\n%s", h.text())
	}
}

func TestSingleReadmeMDOmitted(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/my_project/readme.md", fm("Not Started", ""))

	h := newHarness(t, root, "0")
	h.menu.Select()

	if !strings.Contains(h.text(), "  1) [Not Started] plans/my_project") {
		t.Fatalf("single readme.md should be omitted:\n%s", h.text())
	}
}

func TestSelectNestedByDisplayName(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/parent/plan.md", fm("Not Started", ""))
	plan(t, root, "plans/parent/child/plan.md", fm("Not Started", "subtask_of: plans/parent/plan.md\n"))

	// Selecting by displayed path "plans/parent/child"
	h := newHarness(t, root, "plans/parent/child")
	r := h.menu.Select()
	if r.Task == nil || r.Task.Path != "plans/parent/child/plan.md" {
		t.Fatalf("expected plans/parent/child/plan.md, got %+v", r.Task)
	}

	// Selecting by short name "parent/child"
	h = newHarness(t, root, "parent/child")
	r = h.menu.Select()
	if r.Task == nil || r.Task.Path != "plans/parent/child/plan.md" {
		t.Fatalf("expected plans/parent/child/plan.md, got %+v", r.Task)
	}
}

func TestDeepNestingIndentationFormat(t *testing.T) {
	item0 := Item{Entry: planstatus.Entry{Path: "plans/root/plan.md", Status: "Not Started"}, Depth: 0}
	item1 := Item{Entry: planstatus.Entry{Path: "plans/root/c1/plan.md", Status: "Not Started"}, Depth: 1, IsSubtask: true}
	item2 := Item{Entry: planstatus.Entry{Path: "plans/root/c1/c2/plan.md", Status: "Not Started"}, Depth: 2, IsSubtask: true}
	item3 := Item{Entry: planstatus.Entry{Path: "plans/root/c1/c2/c3/plan.md", Status: "Not Started"}, Depth: 3, IsSubtask: true}

	if got := FormatTaskLine(item0, 9, nil, false); got != "  10) [Not Started] plans/root/plan.md" {
		t.Fatalf("depth 0 got %q", got)
	}
	if got := FormatTaskLine(item1, 10, nil, false); got != "     ↳ 11) [Not Started] plans/root/c1/plan.md" {
		t.Fatalf("depth 1 got %q", got)
	}
	if got := FormatTaskLine(item2, 11, nil, false); got != "        ↳ 12) [Not Started] plans/root/c1/c2/plan.md" {
		t.Fatalf("depth 2 got %q", got)
	}
	if got := FormatTaskLine(item3, 12, nil, false); got != "           ↳ 13) [Not Started] plans/root/c1/c2/c3/plan.md" {
		t.Fatalf("depth 3 got %q", got)
	}
}

func TestDeepAutoInheritance(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/parent/plan.md", fm("Not Started", "auto: true\n"))
	plan(t, root, "plans/parent/child/plan.md", fm("Not Started", "subtask_of: plans/parent/plan.md\n"))
	plan(t, root, "plans/parent/child/grandchild/plan.md", fm("Not Started", "subtask_of: plans/parent/child/plan.md\n"))

	h := newHarness(t, root, "0")
	h.menu.Select()

	if !strings.Contains(h.text(), "  1) [Not Started] plans/parent [auto]") {
		t.Fatalf("parent should be [auto]:\n%s", h.text())
	}
	if !strings.Contains(h.text(), "     ↳ 2) [Not Started] plans/parent/child [auto]") {
		t.Fatalf("child should be [auto]:\n%s", h.text())
	}
	if !strings.Contains(h.text(), "        ↳ 3) [Not Started] plans/parent/child/grandchild [auto]") {
		t.Fatalf("grandchild should be [auto]:\n%s", h.text())
	}
}

