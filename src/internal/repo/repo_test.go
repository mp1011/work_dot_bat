package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func sameDir(t *testing.T, a, b string) {
	t.Helper()
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	if !strings.EqualFold(filepath.Clean(ra), filepath.Clean(rb)) {
		t.Errorf("got %s, want %s", a, b)
	}
}

func TestFindMainRepoRootFromRealLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	main := t.TempDir()
	git(t, main, "init", "-q")
	git(t, main, "commit", "-q", "--allow-empty", "-m", "init")
	tree := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", tree, "-b", "feature")

	sameDir(t, FindMainRepoRoot(main), main)
	sameDir(t, FindMainRepoRoot(tree), main)
}

func TestFindMainRepoRootEdgeCases(t *testing.T) {
	dir := t.TempDir()
	if got := FindMainRepoRoot(dir); got != dir { // no .git at all
		t.Errorf("no .git: %s", got)
	}
	os.WriteFile(filepath.Join(dir, ".git"), []byte("garbage"), 0o644)
	if got := FindMainRepoRoot(dir); got != dir {
		t.Errorf("no gitdir line: %s", got)
	}
	os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /srv/main/.git/worktrees/wt\n"), 0o644)
	if got := filepath.ToSlash(FindMainRepoRoot(dir)); got != "/srv/main" {
		t.Errorf("posix gitdir: %s", got)
	}
	os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../main/.git/worktrees/wt\n"), 0o644)
	want := filepath.Join(filepath.Dir(dir), "main")
	if got := FindMainRepoRoot(dir); filepath.Clean(got) != want {
		t.Errorf("relative gitdir: %s, want %s", got, want)
	}
}

func TestStatePaths(t *testing.T) {
	root := filepath.Join("r", "p")
	for got, want := range map[string]string{
		StateDir(root):   filepath.Join(root, ".work_dot_bat"),
		LedgerPath(root): filepath.Join(root, ".work_dot_bat", "session_ledger.json"),
		LogsDir(root):    filepath.Join(root, ".work_dot_bat", "logs"),
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
}

func TestActivate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(t.TempDir())
	t.Setenv(ProjectRootEnv, "")
	t.Setenv(ProjectNameEnv, "")
	if err := Activate("foo", dir); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	sameDir(t, wd, dir)
	if os.Getenv(ProjectRootEnv) != dir || os.Getenv(ProjectNameEnv) != "foo" {
		t.Error("env not set")
	}
	if err := Activate("x", filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir should fail")
	}
}

func TestUnchangedGuard(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	os.WriteFile(a, []byte("1"), 0o644)
	os.WriteFile(b, []byte("1"), 0o644)
	g := NewUnchangedGuard(a, b, c)
	g.Snapshot()
	if got := g.Changed(); len(got) != 0 {
		t.Errorf("fresh: %v", got)
	}
	os.WriteFile(a, []byte("2"), 0o644)
	os.WriteFile(c, []byte("new"), 0o644)
	if got := g.Changed(); len(got) != 2 || got[0] != a || got[1] != c {
		t.Errorf("changed = %v", got)
	}
}
