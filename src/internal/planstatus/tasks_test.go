package planstatus

import (
	"bytes"
	"os/exec"
	"testing"
)

func taskRoot(t *testing.T) string {
	root := t.TempDir()
	plan(t, root, "plans/a_done.md", "---\nstatus: Done\n---\n\nx\n")
	plan(t, root, "plans/b_active.md", "---\nstatus: In Progress\n---\n\nx\n")
	plan(t, root, "plans/c_free.md", "---\nstatus: Not Started\n---\n\nx\n")
	plan(t, root, "plans/d_cancelled.md", "---\nstatus: Cancelled\n---\n\nx\n")
	return root
}

func listTasks(root string, cli CLI, args ...string) string {
	var o bytes.Buffer
	cli.Root, cli.Stdout, cli.Stderr = root, &o, &o
	cli.Run(append([]string{"list-tasks"}, args...))
	return o.String()
}

func TestListTasksIndexed(t *testing.T) {
	root := taskRoot(t)
	cli := CLI{}
	want := "1. [In Progress] plans/b_active.md\n" +
		"2. [Not Started] plans/c_free.md\n"
	if got := listTasks(root, cli); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := listTasks(root, cli, "--free"); got != "1. [Not Started] plans/c_free.md\n" {
		t.Fatalf("--free got %q", got)
	}
}

func TestListTasksEmptyAndParkedFooter(t *testing.T) {
	root := t.TempDir()
	plan(t, root, "plans/done.md", "---\nstatus: Done\n---\n\nx\n")
	if got := listTasks(root, CLI{}); got != "No open tasks found.\n" {
		t.Fatalf("got %q", got)
	}
	plan(t, root, "plans/agent_suggestions/s.md", "---\nstatus: Not Started\n---\n\nx\n")
	plan(t, root, "plans/open.md", "---\nstatus: Not Started\n---\n\nx\n")
	want := "1. [Not Started] plans/open.md\n1 parked suggestion — scripts/work_dot_bat/plan-status.sh list Suggestion\n"
	if got := listTasks(root, CLI{}); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestListTasksShowsWorktreeAdvancedPlans(t *testing.T) {
	root := taskRoot(t)
	cli := CLI{Collect: func(string) (map[string]WorktreeStatus, error) {
		return map[string]WorktreeStatus{
			"plans/c_free.md":    {Status: "Done", Branch: "feat"}, // finished on a branch: drops out
			"plans/b_active.md": {Status: "Ready for Review", Branch: "feat"},
		}, nil
	}}
	if got := listTasks(root, cli); got != "1. [Ready for Review @ feat] plans/b_active.md\n" {
		t.Fatalf("got %q", got)
	}
}

func TestListChangedDoneAgainstRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	plan(t, root, "plans/a.md", "---\nstatus: In Progress\n---\n\nx\n")
	plan(t, root, "plans/b.md", "---\nstatus: Not Started\n---\n\nx\n")
	g("init", "-q")
	g("add", ".")
	g("commit", "-q", "-m", "base")
	plan(t, root, "plans/a.md", "---\nstatus: Ready for Review\n---\n\nx\n")
	plan(t, root, "plans/b.md", "---\nstatus: In Progress\n---\n\nx\n")

	var o bytes.Buffer
	CLI{Root: root, Stdout: &o, Stderr: &o}.Run([]string{"list-changed-done", "HEAD"})
	if o.String() != "Ready for Review\tplans/a.md\n" {
		t.Fatalf("got %q", o.String())
	}
	o.Reset()
	CLI{Root: root, Stdout: &o, Stderr: &o}.Run([]string{"list-changed-done", "no-such-ref"})
	if o.String() != "Ready for Review\tplans/a.md\n" { // unknown ref: falls back to working-tree status
		t.Fatalf("fallback got %q", o.String())
	}
}
