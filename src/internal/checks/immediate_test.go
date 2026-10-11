package checks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/immediate"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projects"
)

// A real failing command: `go list ./pkg` fails until the package exists.
func immediateEnv(t *testing.T, root string) Env {
	t.Helper()
	put(t, root, "work_dot_bat.config.json", `{"checks":[]}`)
	put(t, root, "go.mod", "module scratch\n\ngo 1.21\n")
	if err := os.MkdirAll(filepath.Join(root, "agent_docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	proj := projects.Project{Name: "p", Path: root, Tests: projects.TestsConfig{
		Light: projects.CommandList{{"go", "list", "./pkg"}},
	}}
	return Env{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, Projects: []projects.Project{proj}}
}

func TestFailedAutomationIsRecordedThenClearedWhenItPasses(t *testing.T) {
	root := t.TempDir()
	e := immediateEnv(t, root)
	name := "[light] go list ./pkg"

	failures := e.RunAutomatedTests(context.Background(), root, root, "light")
	if len(failures) != 1 || failures[0] != name {
		t.Fatalf("failures %v", failures)
	}
	b, err := os.ReadFile(immediate.FilePath(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"status: Not Started", "## AUTOMATION: " + name, "go list ./pkg", "pkg"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}
	if !immediate.Pending(root) {
		t.Fatal("not pending")
	}

	put(t, root, "pkg/a.go", "package pkg\n")
	if failures := e.RunAutomatedTests(context.Background(), root, root, "light"); len(failures) != 0 {
		t.Fatalf("failures %v", failures)
	}
	if st, _ := planstatus.ReadEntryStatus(root, immediate.Plan); st != "Done" || immediate.Pending(root) {
		t.Errorf("status %q", st)
	}
	b, _ = os.ReadFile(immediate.FilePath(root))
	if got := strings.TrimSpace(string(b)); got != "---\nstatus: Done\n---" {
		t.Errorf("not erased: %q", got)
	}
}

func TestWorktreeRunsDoNotTouchTheImmediatePlan(t *testing.T) {
	root := t.TempDir()
	e := immediateEnv(t, root)
	wt := t.TempDir()
	put(t, wt, "work_dot_bat.config.json", `{"checks":[]}`)
	put(t, wt, "go.mod", "module scratch\n\ngo 1.21\n")
	if failures := e.RunAutomatedTests(context.Background(), wt, root, "light"); len(failures) != 1 {
		t.Fatalf("failures %v", failures)
	}
	if immediate.Pending(root) || immediate.Pending(wt) {
		t.Error("a run in a worktree must not record")
	}
}

func TestUnwritableImmediatePlanDoesNotChangeTheResult(t *testing.T) {
	root := t.TempDir()
	e := immediateEnv(t, root)
	// A file where agent_docs/plans must be makes the plan unwritable.
	os.RemoveAll(filepath.Join(root, "agent_docs"))
	put(t, root, "agent_docs", "not a directory")
	var errOut bytes.Buffer
	e.Err = &errOut
	failures := e.RunAutomatedTests(context.Background(), root, root, "light")
	if len(failures) != 1 {
		t.Fatalf("failures %v", failures)
	}
	if !strings.Contains(errOut.String(), "could not update") {
		t.Errorf("no warning: %q", errOut.String())
	}
}

func TestRecordedCheckThatIsNoLongerConfiguredClearsOnAPass(t *testing.T) {
	root := t.TempDir()
	e := immediateEnv(t, root)
	put(t, root, "pkg/a.go", "package pkg\n")
	rec := func(name string) {
		if err := immediate.Record(root, []immediate.Failure{{Name: name, Command: name, Output: "boom"}}); err != nil {
			t.Fatal(err)
		}
	}
	rec("Source Assertion Check")
	rec("[heavy] go test ./...")

	if failures := e.RunAutomatedTests(context.Background(), root, root, "light"); len(failures) != 0 {
		t.Fatalf("failures %v", failures)
	}
	got := immediate.Recorded(root)
	if len(got) != 1 || got[0] != "[heavy] go test ./..." {
		t.Fatalf("recorded %v: the obsolete check should clear, the unrun tier should stay", got)
	}
}
