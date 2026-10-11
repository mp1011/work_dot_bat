package immediate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/planstatus"
)

func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agent_docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func read(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(FilePath(root))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func status(t *testing.T, root string) string {
	t.Helper()
	s, _ := planstatus.ReadEntryStatus(root, Plan)
	return s
}

func TestRecordCreatesPlan(t *testing.T) {
	root := newRoot(t)
	if Pending(root) {
		t.Fatal("pending without a file")
	}
	if err := Record(root, []Failure{{Name: "Build: go build", Command: "go build ./...", Output: "main.go:3: boom"}}); err != nil {
		t.Fatal(err)
	}
	got := read(t, root)
	for _, want := range []string{"status: Not Started", "## AUTOMATION: Build: go build", "go build ./...", "main.go:3: boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if !Pending(root) {
		t.Error("not pending after a failure")
	}
}

func TestRecordMultipleKeepsOneFrontmatter(t *testing.T) {
	root := newRoot(t)
	Record(root, []Failure{{Name: "A", Output: "a1"}})
	Record(root, []Failure{{Name: "B", Output: "b1"}})
	Record(root, []Failure{{Name: "A", Output: "a2"}})
	got := read(t, root)
	if n := strings.Count(got, "status:"); n != 1 {
		t.Errorf("want one frontmatter block, got %d status lines:\n%s", n, got)
	}
	if strings.Count(got, "## AUTOMATION: A") != 1 || strings.Count(got, "## AUTOMATION: B") != 1 {
		t.Errorf("want one section each:\n%s", got)
	}
	if strings.Contains(got, "a1") || !strings.Contains(got, "a2") || !strings.Contains(got, "b1") {
		t.Errorf("repeat failure must replace only its own section:\n%s", got)
	}
}

func TestRecordTailTruncation(t *testing.T) {
	root := newRoot(t)
	out := strings.Repeat("head line\n", 2000) + "the end"
	Record(root, []Failure{{Name: "A", Output: out}})
	got := read(t, root)
	if !strings.Contains(got, "the end") || !strings.Contains(got, "truncated") {
		t.Errorf("tail not kept:\n%s", got[:200])
	}
	if len(got) > MaxOutput+1000 {
		t.Errorf("output not capped: %d bytes", len(got))
	}
}

func TestPendingForEachStatus(t *testing.T) {
	for st, want := range map[string]bool{"Not Started": true, "In Progress": true, "Ready for Review": true, "Blocked - User Input Required": true, "Done": false} {
		root := newRoot(t)
		Record(root, []Failure{{Name: "A"}})
		if st != "Not Started" {
			if err := planstatus.Set(root, Plan, st, false); err != nil {
				t.Fatal(err)
			}
		}
		if got := Pending(root); got != want {
			t.Errorf("%s: Pending=%v want %v", st, got, want)
		}
	}
}

func TestRecordKeepsInProgressButResetsOthers(t *testing.T) {
	root := newRoot(t)
	Record(root, []Failure{{Name: "A"}})
	planstatus.Set(root, Plan, "In Progress", false)
	Record(root, []Failure{{Name: "B"}})
	if s := status(t, root); s != "In Progress" {
		t.Errorf("status %q", s)
	}
	planstatus.Set(root, Plan, "Ready for Review", false)
	Record(root, []Failure{{Name: "B"}})
	if s := status(t, root); s != "Not Started" {
		t.Errorf("status %q", s)
	}
}

func TestClearCompletesAndErases(t *testing.T) {
	root := newRoot(t)
	Record(root, []Failure{{Name: "A", Output: "x"}, {Name: "B", Output: "y"}})
	Clear(root, []string{"Unrelated"})
	if !Pending(root) || !strings.Contains(read(t, root), "## AUTOMATION: A") {
		t.Fatal("unrelated pass must change nothing")
	}
	before := read(t, root)
	Clear(root, []string{"A"})
	got := read(t, root)
	if Pending(root) == false || strings.Contains(got, "AUTOMATION: A") || !strings.Contains(got, "AUTOMATION: B") {
		t.Fatalf("only A should be gone:\n%s\nbefore:\n%s", got, before)
	}
	Clear(root, []string{"B"})
	if status(t, root) != "Done" || Pending(root) {
		t.Errorf("want Done, got %q", status(t, root))
	}
	if got := strings.TrimSpace(read(t, root)); got != "---\nstatus: Done\n---" {
		t.Errorf("body not erased, frontmatter not intact: %q", got)
	}
}

func TestRecordAfterDoneStartsFresh(t *testing.T) {
	root := newRoot(t)
	Record(root, []Failure{{Name: "A", Output: "old"}})
	Clear(root, []string{"A"})
	Record(root, []Failure{{Name: "B", Output: "new"}})
	got := read(t, root)
	if status(t, root) != "Not Started" || strings.Contains(got, "old") || !strings.Contains(got, "new") {
		t.Errorf("status %q:\n%s", status(t, root), got)
	}
	if strings.Count(got, "status:") != 1 {
		t.Errorf("frontmatter duplicated:\n%s", got)
	}
}

func TestRecordFailureToWriteReturnsError(t *testing.T) {
	root := t.TempDir() // no agent_docs dir and a file where it must go
	os.WriteFile(filepath.Join(root, "agent_docs"), []byte("x"), 0o644)
	if err := Record(root, []Failure{{Name: "A"}}); err == nil {
		t.Error("want an error when the plan cannot be written")
	}
}
