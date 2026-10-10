package worktree

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/projects"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repoWithPlans makes <base>/proj with a committed plan.md + subtask, and a gitignored
// local file for the copy step. Returns base and the repo root (forward slashes).
func repoWithPlans(t *testing.T) (base, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base = t.TempDir()
	root = filepath.Join(base, "proj")
	plans := filepath.Join(root, "agent_docs", "plans", "x")
	os.MkdirAll(plans, 0o755)
	os.WriteFile(filepath.Join(plans, "plan.md"), []byte("---\nstatus: In Progress\n---\n\np\n"), 0o644)
	os.WriteFile(filepath.Join(plans, "01.md"), []byte("---\nstatus: Not Started\nsubtask_of: plans/x/plan.md\n---\n\nc\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("local.json\n"), 0o644)
	os.WriteFile(filepath.Join(root, "local.json"), []byte(`{"corpus":"/data"}`), 0o644)
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")
	return base, filepath.ToSlash(root)
}

type recorder struct{ calls []string }

func (r *recorder) run(cmd string, args []string, o RunOpts) (int, error) {
	r.calls = append(r.calls, o.Dir+"|"+cmd+" "+strings.Join(args, " "))
	if cmd == "git" { // really run git so the worktree exists
		return RunCommand(cmd, args, o)
	}
	return 0, nil
}

func TestPathFor(t *testing.T) {
	cases := []struct{ root, branch, name, want string }{
		{"C:/repos/music_mountain", "auto_x", "mm", "C:/repos/mm-auto_x"},
		{`C:\repos\music_mountain\`, "b", "mm", "C:/repos/mm-b"},
		{"C:/repos/music_mountain", "auto_x", "", "C:/repos/music_mountain-auto_x"},
	}
	for _, c := range cases {
		if got := PathFor(c.root, c.branch, c.name); got != c.want {
			t.Errorf("PathFor(%q,%q,%q) = %q want %q", c.root, c.branch, c.name, got, c.want)
		}
	}
}

func TestDecide(t *testing.T) {
	wts := []planstatus.Worktree{{Path: "/a", Branch: "x"}, {Path: "/b", Branch: "gone", Prunable: true}}
	if d := Decide(wts, "x", "/t", false, true); d.Action != Reuse || d.Path != "/a" {
		t.Errorf("%+v", d)
	}
	if d := Decide(wts, "gone", "/t", false, true); d.Action != PruneRequired || !strings.Contains(d.Reason, "git worktree prune") {
		t.Errorf("%+v", d)
	}
	if d := Decide(wts, "n", "/t", true, false); d.Action != Blocked {
		t.Errorf("%+v", d)
	}
	if d := Decide(wts, "n", "/t", false, true); d.Action != AddExistingBranch {
		t.Errorf("%+v", d)
	}
	if d := Decide(wts, "n", "/t", false, false); d.Action != AddNewBranch {
		t.Errorf("%+v", d)
	}
}

func appFor(root, project string, rec *recorder, out *bytes.Buffer, in string) App {
	return App{
		In: bufio.NewReader(strings.NewReader(in)), Out: out, Err: out, Run: rec.run,
		RepoRoot: root, ProjectName: project,
		Provision: func(string, string) (Provisioning, error) {
			return Provisioning{WorktreeCopyFiles: []string{"local.json"}}, nil
		},
	}
}

func TestPlanModeCreatesBranchedWorktreeWithProvisionedFilesAndIsIdempotent(t *testing.T) {
	base, root := repoWithPlans(t)
	var out bytes.Buffer
	rec := &recorder{}
	app := appFor(root, "demo", rec, &out, "")

	if code := app.Main([]string{"plan=plans/x/01.md"}); code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	wt := filepath.Join(base, "demo-x") // the subtask lands on its parent's branch
	if got := git(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); got != "x" {
		t.Fatalf("branch %q", got)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "local.json")); err != nil || string(b) != `{"corpus":"/data"}` {
		t.Fatalf("local.json not provisioned: %v %q", err, b)
	}
	for _, want := range []string{"Plan:     plans/x/01.md", "Subtask of plans/x/plan.md", "Branch:   x", "Creating worktree at ", "Copied local.json to worktree.", "Worktree ready."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if list := git(t, root, "worktree", "list", "--porcelain"); !strings.Contains(list, "branch refs/heads/x") {
		t.Fatalf("not registered:\n%s", list)
	}

	// Re-running reuses it: success, no second creation, no second bootstrap.
	out.Reset()
	before := len(rec.calls)
	if code := app.Main([]string{"plan=plans/x/01.md"}); code != 0 {
		t.Fatalf("rerun code %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Worktree already exists — reusing ") || strings.Contains(out.String(), "Worktree ready.") {
		t.Fatalf("rerun output:\n%s", out.String())
	}
	if len(rec.calls) != before {
		t.Fatalf("rerun ran commands: %v", rec.calls[before:])
	}
}

func TestBootstrapRunsInstallInWorktreeAndStopsOnFailure(t *testing.T) {
	dir := t.TempDir()
	var calls []RunOpts
	run := func(cmd string, args []string, o RunOpts) (int, error) { calls = append(calls, o); return 1, nil }
	var logs []string
	ok := Bootstrap(dir, Provisioning{Install: &installCmd}, run, func(s string) { logs = append(logs, s) })
	if ok || len(calls) != 1 || calls[0].Dir != dir || !strings.Contains(strings.Join(logs, "\n"), "[ERROR] npm install failed with exit code 1") {
		t.Fatalf("ok=%v calls=%+v logs=%v", ok, calls, logs)
	}
}

func TestBootstrapNeverOverwritesExistingFile(t *testing.T) {
	base, root := repoWithPlans(t)
	wt := filepath.Join(base, "wt")
	git(t, root, "worktree", "add", "-q", wt, "-b", "feat")
	os.WriteFile(filepath.Join(wt, "local.json"), []byte("mine"), 0o644)
	Bootstrap(wt, Provisioning{WorktreeCopyFiles: []string{"local.json", "absent.json"}}, nil, func(string) {})
	if b, _ := os.ReadFile(filepath.Join(wt, "local.json")); string(b) != "mine" {
		t.Fatalf("overwrote: %q", b)
	}
}

func TestManualModeAndBlockedDirectory(t *testing.T) {
	base, root := repoWithPlans(t)
	var out bytes.Buffer
	rec := &recorder{}
	if code := appFor(root, "demo", rec, &out, "1\nfeature-a\n").Main(nil); code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Worktree created successfully.") {
		t.Fatalf("%s", out.String())
	}
	if got := git(t, filepath.Join(base, "demo-feature-a"), "rev-parse", "--abbrev-ref", "HEAD"); got != "feature-a" {
		t.Fatalf("branch %q", got)
	}

	// A directory that exists but is not a registered worktree is never touched or bootstrapped.
	os.MkdirAll(filepath.Join(base, "demo-other"), 0o755)
	out.Reset()
	rec.calls = nil
	if code := appFor(root, "demo", rec, &out, "n\n").Main([]string{"branch=other"}); code != 0 || !strings.Contains(out.String(), "already exists") || len(rec.calls) != 0 {
		t.Fatalf("code %d calls %v\n%s", code, rec.calls, out.String())
	}
	out.Reset()
}

func TestStaleRegistrationNeedsPrune(t *testing.T) {
	base, root := repoWithPlans(t)
	wt := filepath.Join(base, "demo-gone")
	git(t, root, "worktree", "add", "-q", wt, "-b", "gone")
	os.RemoveAll(wt)
	var out bytes.Buffer
	if code := appFor(root, "demo", &recorder{}, &out, "").Main([]string{"branch=gone"}); code != 1 || !strings.Contains(out.String(), "git worktree prune") {
		t.Fatalf("code %d\n%s", code, out.String())
	}
}

func TestWorktreeInfoMainVersusLinked(t *testing.T) {
	base, root := repoWithPlans(t)
	wt := filepath.Join(base, "linked")
	git(t, root, "worktree", "add", "-q", wt, "-b", "feat")

	var out, errb bytes.Buffer
	if code := RunInfo(root, &out, &errb, nil); code != 0 {
		t.Fatal(errb.String())
	}
	lines := strings.Split(out.String(), "\n")
	if lines[0] != "worktree      "+root || lines[1] != "branch        main" || !strings.HasPrefix(lines[2], "main checkout ") || lines[3] != "same as main  yes" {
		t.Fatalf("main:\n%s", out.String())
	}
	out.Reset()
	RunInfo(wt, &out, &errb, nil)
	lines = strings.Split(out.String(), "\n")
	if lines[1] != "branch        feat" || lines[3] != "same as main  no" || !strings.EqualFold(strings.TrimPrefix(lines[2], "main checkout "), root) {
		t.Fatalf("linked:\n%s", out.String())
	}
}

func TestDevServerNoneStartsNothing(t *testing.T) {
	if pid, err := StartDevServer(t.TempDir(), Provisioning{}, func(string) {}); pid != 0 || err != nil {
		t.Fatalf("%d %v", pid, err)
	}
}

var installCmd = projectconfig.Command{Cmd: "npm", Args: []string{"install"}}

func TestLoadProvisioningFromProjectsJSON(t *testing.T) {
	regDir := t.TempDir()
	regFile := filepath.Join(regDir, "projects.json")
	t.Setenv(projects.RegistryEnv, regFile)

	root := t.TempDir()
	installCmd := projects.Command{Cmd: "npm", Args: []string{"ci"}}
	devCmd := projects.Command{Cmd: "npm", Args: []string{"run", "dev:test"}}
	projs := []projects.Project{
		{
			Name:              "myrepo",
			Path:              root,
			Install:           &installCmd,
			DevServer:         &devCmd,
			WorktreeCopyFiles: []string{".env.local"},
		},
	}
	if err := projects.SaveRegistry(regFile, projs); err != nil {
		t.Fatal(err)
	}

	p, err := LoadProvisioning(root, root)
	if err != nil {
		t.Fatalf("LoadProvisioning failed: %v", err)
	}
	if p.Install == nil || p.Install.Cmd != "npm" || len(p.Install.Args) != 1 || p.Install.Args[0] != "ci" {
		t.Errorf("p.Install = %+v", p.Install)
	}
	if p.DevServer == nil || p.DevServer.Cmd != "npm" || len(p.DevServer.Args) != 2 || p.DevServer.Args[1] != "dev:test" {
		t.Errorf("p.DevServer = %+v", p.DevServer)
	}
	if len(p.WorktreeCopyFiles) != 1 || p.WorktreeCopyFiles[0] != ".env.local" {
		t.Errorf("p.WorktreeCopyFiles = %+v", p.WorktreeCopyFiles)
	}
}
