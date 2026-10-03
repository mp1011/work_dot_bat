package work

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"work_dot_bat/internal/planstatus"
)

func keys(opts []Option) []string {
	var k []string
	for _, o := range opts {
		k = append(k, o.Key)
	}
	return k
}

func TestTaskMenuOptions(t *testing.T) {
	none, nav := TaskMenuOptions(nil, false, false)
	if !reflect.DeepEqual(keys(none), []string{"c", "p", "t"}) || !reflect.DeepEqual(keys(nav), []string{"s", "q"}) || nav[0].Label != "Select task" {
		t.Errorf("no task: %v %v", none, nav)
	}
	cases := []struct {
		name     string
		task     planstatus.Entry
		worktree bool
		parent   bool
		want     string
	}{
		{"not started", planstatus.Entry{Status: "Not Started"}, false, false, "w g c t a"},
		{"not started auto", planstatus.Entry{Status: "Not Started", Auto: true}, false, false, "w g c t"},
		{"suggestion", planstatus.Entry{Status: "Suggestion"}, false, false, "w g c t a"},
		{"in progress", planstatus.Entry{Status: "In Progress"}, false, false, "w g c t"},
		{"ready", planstatus.Entry{Status: "Ready for Review"}, false, false, "w g d c t"},
		{"ready worktree", planstatus.Entry{Status: "Ready for Review"}, true, false, "w g d c m t"},
		{"in progress worktree", planstatus.Entry{Status: "In Progress"}, true, false, "w g c t"},
		{"done", planstatus.Entry{Status: "Done"}, false, false, "w g t"},
		{"done worktree", planstatus.Entry{Status: "Done"}, true, false, "w g m t"},
		{"cancelled", planstatus.Entry{Status: "Cancelled"}, false, false, "w g t"},
		{"parent", planstatus.Entry{Status: "In Progress"}, false, true, "w l g c t"},
	}
	for _, c := range cases {
		task := c.task
		actions, nav := TaskMenuOptions(&task, c.worktree, c.parent)
		if got := strings.Join(keys(actions), " "); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
		if nav[0].Label != "Start new task" {
			t.Errorf("%s: nav %v", c.name, nav)
		}
	}
	leaf, _ := TaskMenuOptions(&planstatus.Entry{Status: "In Progress"}, false, false)
	parent, _ := TaskMenuOptions(&planstatus.Entry{Status: "In Progress"}, false, true)
	if leaf[0].Label != "Work on Task" || parent[0].Label != "Work on Next Subtask" {
		t.Errorf("%v %v", leaf[0], parent[0])
	}
}

func TestInsertAutoFlag(t *testing.T) {
	cases := []struct{ in, want string }{
		{"---\nstatus: Not Started\n---\n\n# GOAL\ndo the thing\n", "---\nstatus: Not Started\nauto: true\n---\n\n# GOAL\ndo the thing\n"},
		{"---\nstatus: Not Started\nsubtask_of: plans/foo/plan.md\n---\nbody\n", "---\nstatus: Not Started\nauto: true\nsubtask_of: plans/foo/plan.md\n---\nbody\n"},
		{"---\nstatus: Not Started\nauto: true\n---\nbody\n", "---\nstatus: Not Started\nauto: true\n---\nbody\n"},
		{"---\nsubtask_of: plans/foo/plan.md\n---\nbody\n", "---\nsubtask_of: plans/foo/plan.md\nauto: true\n---\nbody\n"},
		{"---\r\nstatus: Not Started\r\n---\r\n\r\n# GOAL\r\nx\r\n", "---\r\nstatus: Not Started\r\nauto: true\r\n---\r\n\r\n# GOAL\r\nx\r\n"},
		{"---\r\nstatus: Not Started\r\nauto: true\r\n---\r\nbody\r\n", "---\r\nstatus: Not Started\r\nauto: true\r\n---\r\nbody\r\n"},
	}
	for _, c := range cases {
		got, err := InsertAutoFlag(c.in)
		if err != nil || got != c.want {
			t.Errorf("%q: got %q err %v", c.in, got, err)
		}
	}
	if _, err := InsertAutoFlag("# GOAL\nno frontmatter here\n"); err == nil || !strings.Contains(err.Error(), "no frontmatter block") {
		t.Errorf("err %v", err)
	}
}

func TestCommitAutoFlag(t *testing.T) {
	rec := func(codes ...int) (Runner, *[][]string) {
		var calls [][]string
		i := 0
		return func(cmd string, args ...string) int {
			calls = append(calls, append([]string{cmd}, args...))
			code := 0
			if i < len(codes) {
				code = codes[i]
			}
			i++
			return code
		}, &calls
	}
	run, calls := rec(0, 0)
	if got := CommitAutoFlag("plans/foo/plan.md", run); got != "committed" {
		t.Error(got)
	}
	want := [][]string{{"git", "add", "plans/foo/plan.md"}, {"git", "commit", "-m", "Mark plans/foo/plan.md auto: true"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("%v", *calls)
	}
	run, calls = rec(1)
	if got := CommitAutoFlag("p.md", run); got != "add-failed" || len(*calls) != 1 {
		t.Error(got)
	}
	run, calls = rec(0, 1)
	if got := CommitAutoFlag("p.md", run); got != "commit-failed" || len(*calls) != 2 {
		t.Error(got)
	}
}

func TestIsStatusOnlyPath(t *testing.T) {
	for _, p := range []string{"agent_docs/plans/september_2026/foo/01_bar.md", "agent_docs/plan_status.js", `agent_docs\plans\foo\bar.md`, `"agent_docs/plans/foo/bar.md"`} {
		if !IsStatusOnlyPath(p) {
			t.Errorf("%q should be status-only", p)
		}
	}
	for _, p := range []string{"vite.config.ts", "src/hooks/useHomepageSong.ts", "agent_docs/deep_dives/auto_mode_daemon.md"} {
		if IsStatusOnlyPath(p) {
			t.Errorf("%q should not be status-only", p)
		}
	}
}

func status(out string) StatusFunc { return func(string) (string, error) { return out, nil } }

func TestTreeHasNonStatusChanges(t *testing.T) {
	f := func(out string) bool { return TreeHasNonStatusChanges("", "", status(out), nil) }
	if f(" M agent_docs/plans/august_2026/page_load_performance/02_vendor_chunk_splitting.md\n M agent_docs/plan_status.js\n") {
		t.Error("plan files only should be false")
	}
	if !f(" M agent_docs/plans/x/01.md\n M vite.config.ts\n") {
		t.Error("source file alongside plans should be true")
	}
	if !f("?? src/newThing.ts\n") {
		t.Error("untracked source should be true")
	}
	if !f("R  agent_docs/plans/x/01.md -> src/moved.ts\n") || f("R  src/old.ts -> agent_docs/plans/x/02.md\n") {
		t.Error("rename should follow the destination")
	}
	if f("") {
		t.Error("clean tree should be false")
	}
	if !TreeHasNonStatusChanges("", "", func(string) (string, error) { return "", errors.New("git missing") }, nil) {
		t.Error("unknowable tree should be true")
	}
	read := func(string) (string, error) {
		return "---\nstatus: Not Started\nsubtask_of: plans/x/parent.md\n---\n\n# GOAL\nDo part 1\n", nil
	}
	out := " M agent_docs/plans/x/parent.md\n M agent_docs/plan_status.js\nA  agent_docs/plans/x/01.md\n"
	if !TreeHasNonStatusChanges("", "", status(out), read) {
		t.Error("a valid breakdown should be true")
	}
}

func TestHasNonEmptyGoal(t *testing.T) {
	for _, s := range []string{"# GOAL\nImplement feature\n", "---\nstatus: Not Started\n---\n\n# Goal\nSome goal\n## Next\n"} {
		if !HasNonEmptyGoal(s) {
			t.Errorf("%q", s)
		}
	}
	for _, s := range []string{"", "# Context\nNo goal here", "# GOAL\n\n## Context\nNo content under goal", "# GOAL\n   \n\n"} {
		if HasNonEmptyGoal(s) {
			t.Errorf("%q", s)
		}
	}
}

func TestPlanBreakdownInfo(t *testing.T) {
	const parentPath, parentRel = "agent_docs/plans/august_2026/foo.md", "plans/august_2026/foo.md"
	sub := func(parent, body string) string {
		return "---\nstatus: Not Started\nsubtask_of: " + parent + "\n---\n\n" + body
	}
	files := map[string]string{
		"agent_docs/plans/august_2026/foo/01_a.md": sub(parentRel, "# GOAL\nSubtask 1 goal\n"),
		"agent_docs/plans/august_2026/foo/02_b.md": sub(parentRel, "# GOAL\nSubtask 2 goal\n"),
	}
	read := func(p string) (string, error) {
		if c, ok := files[strings.ReplaceAll(p, `\`, "/")]; ok {
			return c, nil
		}
		return "", errors.New("missing")
	}
	ok := PlanBreakdownInfo([]string{" M " + parentPath, " M agent_docs/plan_status.js",
		"A  agent_docs/plans/august_2026/foo/01_a.md", "A  agent_docs/plans/august_2026/foo/02_b.md"}, read, "")
	if !ok.IsBreakdown || len(ok.Subtasks) != 2 {
		t.Errorf("%+v", ok)
	}
	if b := PlanBreakdownInfo([]string{" M " + parentPath, " M agent_docs/plan_status.js"}, read, ""); b.IsBreakdown || b.Reason != "no added subtask files" {
		t.Errorf("bookkeeping only: %+v", b)
	}
	files["agent_docs/plans/august_2026/foo/01_a.md"] = sub(parentRel, "# GOAL\n")
	if b := PlanBreakdownInfo([]string{"A  agent_docs/plans/august_2026/foo/01_a.md"}, read, ""); b.IsBreakdown || !strings.Contains(b.Reason, "empty goal") {
		t.Errorf("empty goal: %+v", b)
	}
	files["agent_docs/plans/august_2026/foo/01_a.md"] = "---\nstatus: Not Started\n---\n\n# GOAL\nStray\n"
	if b := PlanBreakdownInfo([]string{"A  agent_docs/plans/august_2026/foo/01_a.md"}, read, ""); b.IsBreakdown || !strings.Contains(b.Reason, "missing subtask_of") {
		t.Errorf("stray: %+v", b)
	}
	files["agent_docs/plans/august_2026/foo/01_a.md"] = sub("plans/other.md", "# GOAL\nGoal\n")
	if b := PlanBreakdownInfo([]string{" M " + parentPath, "A  agent_docs/plans/august_2026/foo/01_a.md"}, read, parentRel); b.IsBreakdown || !strings.Contains(b.Reason, "does not match expected parent") {
		t.Errorf("wrong parent: %+v", b)
	}
	if b := PlanBreakdownInfo([]string{"D  agent_docs/plans/x.md"}, read, ""); b.IsBreakdown || !strings.Contains(b.Reason, "deleted file") {
		t.Errorf("delete: %+v", b)
	}
	if b := PlanBreakdownInfo(nil, read, ""); b.IsBreakdown || b.Reason != "empty change list" {
		t.Errorf("empty: %+v", b)
	}
}
