package work

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/taskmenu"
	"work_dot_bat/src/internal/worktree"
)

func writePlan(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, "agent_docs", filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	t      *testing.T
	root   string
	out    bytes.Buffer
	inputs []string
	agent  []agentcmd.Command // commands the fake agent was given
	gitRun [][]string
	app    App
	// onAgent runs inside the fake agent, while the claim should be held.
	onAgent func(ctx context.Context) int
}

func newFixture(t *testing.T, inputs ...string) *fixture {
	root := t.TempDir()
	writePlan(t, root, "plans/x.md", "---\nstatus: Not Started\n---\n\n# GOAL\nDo x\n")
	f := &fixture{t: t, root: root, inputs: inputs}
	f.app = App{
		Root: root, Out: &f.out,
		Ask: func(prompt string) (string, bool) {
			f.out.WriteString(prompt)
			if len(f.inputs) == 0 {
				return "", false
			}
			in := f.inputs[0]
			f.inputs = f.inputs[1:]
			return in, true
		},
		ClaimEnv:  planclaims.DefaultEnv(),
		Dirs:      agentsession.Dirs{Projects: t.TempDir(), AgyBrain: t.TempDir()},
		Settings:  agentcmd.DefaultSettings,
		Worktrees: func() []planstatus.Worktree { return nil },
		Tasks:     func() []planstatus.Entry { return planstatus.ReadStatus(root) },
		Guard:     func() func() { return func() {} },
		Git: func(cmd string, args []string, o worktree.RunOpts) (int, error) {
			f.gitRun = append(f.gitRun, append([]string{cmd}, args...))
			return 0, nil
		},
		RunAgent: func(ctx context.Context, c agentcmd.Command, dir string) (int, error) {
			f.agent = append(f.agent, c)
			if f.onAgent != nil {
				return f.onAgent(ctx), nil
			}
			return 0, nil
		},
	}
	return f
}

func (f *fixture) claimFiles() []string {
	entries, _ := os.ReadDir(repo.ClaimsDir(f.root))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (f *fixture) selecting(path string) {
	f.app.Pick = func() taskmenu.Result {
		return taskmenu.Result{Task: &taskmenu.Item{Entry: planstatus.Entry{Path: path, Status: "Not Started"}}}
	}
}

func (f *fixture) run() int {
	// After the first pick the second selection ends the run.
	picks := 0
	pick := f.app.Pick
	f.app.Pick = func() taskmenu.Result {
		picks++
		if picks > 1 {
			return taskmenu.Result{Quit: true}
		}
		return pick()
	}
	return f.app.Run(context.Background())
}

func planStatusOf(t *testing.T, root, rel string) string {
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs", filepath.FromSlash(rel)))
	fm := planstatus.ParseFrontmatter(string(b))
	if fm.Status == nil {
		return ""
	}
	return *fm.Status
}

// Work on a leaf plan: claim held during the agent run, prompt carries the launcher note, and
// the claim is gone afterwards even though the agent failed.
func TestWorkOnLeafHoldsClaimAndReleasesAfterFailingAgent(t *testing.T) {
	f := newFixture(t, "w", "be careful", "c", "n", "s")
	f.selecting("plans/x.md")
	var during []string
	f.onAgent = func(context.Context) int { during = f.claimFiles(); return 1 }
	if code := f.run(); code != 0 {
		t.Fatalf("exit %d\n%s", code, f.out.String())
	}
	if len(during) != 2 {
		t.Errorf("plan and branch claims should be held during the run, got %v", during)
	}
	if left := f.claimFiles(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
	if len(f.agent) != 1 {
		t.Fatalf("agent runs: %d\n%s", len(f.agent), f.out.String())
	}
	c := f.agent[0]
	wantPrompt := "/do-work-yolo agent_docs/plans/x.md be careful" + agentcmd.LauncherClaimNote
	if c.Cmd != "claude" || c.Args[len(c.Args)-1] != wantPrompt || contains(c.Args, "--print") {
		t.Errorf("command %+v", c)
	}
	if got := planStatusOf(t, f.root, "plans/x.md"); got != "In Progress" {
		t.Errorf("status %q", got)
	}
	ledger := agentsession.ReadLedger(repo.LedgerPath(f.root))
	if len(ledger) != 1 || ledger[0]["status"] != "failed" || ledger[0]["exitCode"] != float64(1) || ledger[0]["plan"] != "agent_docs/plans/x.md" {
		t.Errorf("ledger %v", ledger)
	}
	for _, want := range []string{"Session Name: ", "Launching Claude Code interactively with prompt:", "[Agent session completed]", "Do you want to run automated tests? (y/n): "} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, f.out.String())
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// An interrupt (the context is cancelled while the agent runs) still releases the claim.
func TestInterruptReleasesClaim(t *testing.T) {
	f := newFixture(t, "w", "", "c", "n", "s")
	f.selecting("plans/x.md")
	ctx, cancel := context.WithCancel(context.Background())
	f.onAgent = func(ctx context.Context) int {
		if len(f.claimFiles()) != 2 {
			t.Errorf("claims not held before launch: %v", f.claimFiles())
		}
		cancel()
		<-ctx.Done()
		return 1
	}
	picks := 0
	f.app.Pick = func() taskmenu.Result {
		picks++
		if picks > 1 {
			return taskmenu.Result{Quit: true}
		}
		return taskmenu.Result{Task: &taskmenu.Item{Entry: planstatus.Entry{Path: "plans/x.md", Status: "Not Started"}}}
	}
	f.app.Run(ctx)
	if left := f.claimFiles(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
}

func TestClaimReleasedWhenLaunchPanics(t *testing.T) {
	f := newFixture(t)
	f.app.RunAgent = func(context.Context, agentcmd.Command, string) (int, error) { panic("boom") }
	func() {
		defer func() { recover() }()
		f.app.WithClaim("agent_docs/plans/x.md", func() {
			f.app.LaunchInteractiveAgent(context.Background(), "c", "agent_docs/plans/x.md", "p")
		})
	}()
	if left := f.claimFiles(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
}

func TestClaimHeldBySomeoneElseSkipsTheRun(t *testing.T) {
	f := newFixture(t, "w", "", "c", "s")
	f.selecting("plans/x.md")
	other := planclaims.CLI{Root: f.root, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Env: planclaims.DefaultEnv()}
	if code := other.Run([]string{"acquire", "plans/x.md", "--owner", "daemon", "--kind", "daemon", "--pid", strconv.Itoa(os.Getppid())}); code != 0 {
		t.Fatal("could not set up the held claim")
	}
	f.run()
	if len(f.agent) != 0 {
		t.Error("agent ran despite the held claim")
	}
	if !strings.Contains(f.out.String(), "[SKIP] The plan/branch claim is held by someone else. Not starting this task.") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if len(f.claimFiles()) != 2 {
		t.Errorf("the other holder's claim must stay: %v", f.claimFiles())
	}
}

func TestParentPlanRunsNextSubtaskWithoutTakingAClaim(t *testing.T) {
	f := newFixture(t, "w", "", "c", "n", "s")
	writePlan(t, f.root, "plans/p/plan.md", "---\nstatus: In Progress\n---\n\n# GOAL\nparent\n")
	writePlan(t, f.root, "plans/p/01.md", "---\nstatus: Not Started\nsubtask_of: plans/p/plan.md\n---\n\n# GOAL\nchild\n")
	f.selecting("plans/p/plan.md")
	var during []string
	f.onAgent = func(context.Context) int { during = f.claimFiles(); return 0 }
	f.run()
	if len(f.agent) != 1 {
		t.Fatalf("agent runs: %d\n%s", len(f.agent), f.out.String())
	}
	if got := f.agent[0].Args[len(f.agent[0].Args)-1]; got != "/next-subtask-yolo agent_docs/plans/p/plan.md" {
		t.Errorf("prompt %q", got)
	}
	if len(during) != 0 {
		t.Errorf("the launcher must not hold a claim for a parent plan: %v", during)
	}
	if !strings.Contains(f.out.String(), "w) Work on Next Subtask") || !strings.Contains(f.out.String(), "l) Loop until done") {
		t.Errorf("menu:\n%s", f.out.String())
	}
}

func TestTaskMenuGolden(t *testing.T) {
	f := newFixture(t, "s")
	f.selecting("plans/x.md")
	f.run()
	want := "\n---------------------------------------------------\n" +
		"Task: plans/x.md\n" +
		"Current Status: [Not Started]\n" +
		"---------------------------------------------------\n" +
		"  w) Work on Task\n  g) General agent request\n  c) Cancel task\n  t) Run automated tests\n  a) Set auto: true (commits)\n\n" +
		"  s) Start new task\n  q) Quit\n\n" +
		"Select option (w/g/c/t/a/s/q): \nReturning to task selection...\n"
	if !strings.Contains(f.out.String(), want) {
		t.Errorf("got:\n%s", f.out.String())
	}
}


func TestGeneralRequestUsesPlanContextPrompt(t *testing.T) {
	f := newFixture(t, "g", "look at the tests", "a", "n", "s")
	f.selecting("plans/x.md")
	f.run()
	if len(f.agent) != 1 {
		t.Fatalf("%d agent runs\n%s", len(f.agent), f.out.String())
	}
	c := f.agent[0]
	if c.Cmd != "agy" || c.Args[len(c.Args)-2] != "-i" ||
		c.Args[len(c.Args)-1] != "Read the plan file at agent_docs/plans/x.md for context.\n\nlook at the tests" {
		t.Errorf("%+v", c)
	}
	if len(f.claimFiles()) != 0 {
		t.Error("general requests take no claim")
	}
}

func TestCancelAndMarkDone(t *testing.T) {
	f := newFixture(t, "c", "no longer needed", "s")
	f.selecting("plans/x.md")
	f.run()
	if got := planStatusOf(t, f.root, "plans/x.md"); got != "Cancelled" {
		t.Errorf("status %q", got)
	}
	b, _ := os.ReadFile(filepath.Join(f.root, "agent_docs/plans/x.md"))
	if !strings.HasSuffix(string(b), "\n# CANCELLED\nJustification: no longer needed\n") {
		t.Errorf("file:\n%s", b)
	}

	g := newFixture(t, "d", "s")
	writePlan(t, g.root, "plans/x.md", "---\nstatus: Ready for Review\n---\n\n# GOAL\nx\n")
	g.selecting("plans/x.md")
	g.run()
	if got := planStatusOf(t, g.root, "plans/x.md"); got != "Done" {
		t.Errorf("status %q\n%s", got, g.out.String())
	}

	h := newFixture(t, "d", "s")
	h.selecting("plans/x.md")
	h.run()
	// 'd' is not offered for a Not Started task, but typing it is answered like Node does.
	if !strings.Contains(h.out.String(), "Task must be \"Ready for Review\" to mark as Done.") {
		t.Errorf("%s", h.out.String())
	}
}

func TestSetAutoStagesOnlyThePlanFile(t *testing.T) {
	f := newFixture(t, "a", "y", "s")
	f.selecting("plans/x.md")
	f.run()
	b, _ := os.ReadFile(filepath.Join(f.root, "agent_docs/plans/x.md"))
	if !strings.HasPrefix(string(b), "---\nstatus: Not Started\nauto: true\n---") {
		t.Errorf("file:\n%s", b)
	}
	want := [][]string{{"git", "add", "agent_docs/plans/x.md"}, {"git", "commit", "-m", "Mark agent_docs/plans/x.md auto: true"}}
	if len(f.gitRun) != 2 || strings.Join(f.gitRun[0], " ") != strings.Join(want[0], " ") || strings.Join(f.gitRun[1], " ") != strings.Join(want[1], " ") {
		t.Errorf("git calls %v", f.gitRun)
	}

	g := newFixture(t, "a", "n", "s")
	g.selecting("plans/x.md")
	g.run()
	if len(g.gitRun) != 0 || strings.Contains(readFile(g.root, "plans/x.md"), "auto:") {
		t.Error("declined confirmation must change nothing")
	}
}

func readFile(root, rel string) string {
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs", filepath.FromSlash(rel)))
	return string(b)
}

func TestNoTaskMenuCommit(t *testing.T) {
	f := newFixture(t, "c", "my message", "s")
	f.app.Pick = func() taskmenu.Result { return taskmenu.Result{} }
	picks := 0
	pick := f.app.Pick
	f.app.Pick = func() taskmenu.Result {
		picks++
		if picks > 1 {
			return taskmenu.Result{Quit: true}
		}
		return pick()
	}
	f.app.Run(context.Background())
	want := "git add -A|git commit -m my message"
	var got []string
	for _, c := range f.gitRun {
		got = append(got, strings.Join(c, " "))
	}
	if strings.Join(got, "|") != want {
		t.Errorf("git calls %v", got)
	}
	if !strings.Contains(f.out.String(), "Task: None selected") || !strings.Contains(f.out.String(), "Select option (c/p/t/s/q): ") {
		t.Errorf("%s", f.out.String())
	}
}

func TestTestMenuSelection(t *testing.T) {
	cases := []struct {
		input     string
		wantLayer string
	}{
		{"u", "ultra_light"},
		{"0", "ultra_light"},
		{"1", "light"},
		{"l", "light"},
		{"2", "heavy"},
		{"h", "heavy"},
		{"3", "ultra_heavy"},
		{"p", "ultra_heavy"},
	}
	for _, c := range cases {
		var calledWith []string
		f := newFixture(t, "t", c.input, "s")
		f.app.RunTests = func(layers ...string) []string {
			calledWith = layers
			return nil
		}
		f.selecting("plans/x.md")
		f.run()
		if len(calledWith) != 1 || calledWith[0] != c.wantLayer {
			t.Errorf("input %q: got layers %v, want [%s]", c.input, calledWith, c.wantLayer)
		}
		if !strings.Contains(f.out.String(), "Select Test Level:") || !strings.Contains(f.out.String(), "Select option (u/1/2/3/b): ") {
			t.Errorf("input %q: menu output:\n%s", c.input, f.out.String())
		}
	}
}

func TestTestMenuBack(t *testing.T) {
	var called bool
	f := newFixture(t, "t", "b", "s")
	f.app.RunTests = func(layers ...string) []string {
		called = true
		return nil
	}
	f.selecting("plans/x.md")
	f.run()
	if called {
		t.Errorf("RunTests was called when choosing back")
	}
}

func TestNoTaskTestMenu(t *testing.T) {
	var calledWith []string
	f := newFixture(t, "t", "1", "s")
	f.app.RunTests = func(layers ...string) []string {
		calledWith = layers
		return nil
	}
	f.app.Pick = func() taskmenu.Result {
		return taskmenu.Result{}
	}
	f.run()
	if len(calledWith) != 1 || calledWith[0] != "light" {
		t.Errorf("no task test menu: got %v, want [light]", calledWith)
	}
}

func TestPostSessionRunTestsShowsTestMenu(t *testing.T) {
	var calledWith []string
	f := newFixture(t, "w", "", "c", "y", "1", "s")
	f.app.RunTests = func(layers ...string) []string {
		calledWith = layers
		return nil
	}
	f.selecting("plans/x.md")
	f.run()
	if len(calledWith) != 1 || calledWith[0] != "light" {
		t.Errorf("got %v, want [light]", calledWith)
	}
	if !strings.Contains(f.out.String(), "Do you want to run automated tests? (y/n): ") ||
		!strings.Contains(f.out.String(), "Select Test Level:") {
		t.Errorf("output missing prompt or test menu:\n%s", f.out.String())
	}
}

func TestEndOfInputQuits(t *testing.T) {
	f := newFixture(t)
	f.selecting("plans/x.md")
	if code := f.app.Run(context.Background()); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestFormatWorktreeSummary(t *testing.T) {
	got := FormatWorktreeSummary([]planstatus.Worktree{
		{Path: "C:/r/proj", Branch: "main"},
		{Path: "C:/r/proj-x", Branch: "x", Prunable: true},
	}, `C:\r\proj`)
	if len(got) != 2 || got[0] != "- [main] C:/r/proj (main repository)" || got[1] != "- [x] C:/r/proj-x [prunable]" {
		t.Errorf("%v", got)
	}
}

// --- the real agent runner, against a stub executable (this test binary re-executed) ---

func TestHelperAgent(t *testing.T) {
	switch os.Getenv("WDB_HELPER") {
	case "exit7":
		os.Exit(7)
	case "sleep":
		time.Sleep(time.Minute)
	case "echo":
		// Reads one line from the stdin it was given and echoes it: the launcher must pass
		// the terminal through without swallowing input.
		buf := make([]byte, 64)
		n, _ := os.Stdin.Read(buf)
		os.Stdout.WriteString("GOT:" + strings.TrimSpace(string(buf[:n])))
		os.Exit(0)
	}
}

func helperCmd(mode string) agentcmd.Command {
	return agentcmd.Command{Cmd: os.Args[0], Args: []string{"-test.run=^TestHelperAgent$"}}
}

func TestExecAgentExitCodeAndCancel(t *testing.T) {
	t.Setenv("WDB_HELPER", "exit7")
	if code, err := ExecAgent(context.Background(), helperCmd("exit7"), t.TempDir()); err != nil || code != 7 {
		t.Errorf("code %d err %v", code, err)
	}

	t.Setenv("WDB_HELPER", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, err := ExecAgent(ctx, helperCmd("sleep"), t.TempDir())
	if err != nil || code == 0 || time.Since(start) > 30*time.Second {
		t.Errorf("a cancelled agent must be killed and report failure: code %d err %v after %v", code, err, time.Since(start))
	}

	if code, _ := ExecAgent(context.Background(), agentcmd.Command{Cmd: "definitely-not-a-real-binary-xyz"}, t.TempDir()); code != 1 {
		t.Errorf("a missing binary should report 1, got %d", code)
	}
}

func TestExecAgentPassesStdinThrough(t *testing.T) {
	t.Setenv("WDB_HELPER", "echo")
	in, w, _ := os.Pipe()
	out, outW, _ := os.Pipe()
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, outW
	w.WriteString("hello agent\n")
	code, err := ExecAgent(context.Background(), helperCmd("echo"), t.TempDir())
	os.Stdin, os.Stdout = oldIn, oldOut
	outW.Close()
	var got bytes.Buffer
	got.ReadFrom(out)
	if err != nil || code != 0 || !strings.Contains(got.String(), "GOT:hello agent") {
		t.Errorf("code %d err %v out %q", code, err, got.String())
	}
}

var _ = exec.Command

// Loop mode through the launcher: subtasks then parent, non-interactive (--print) runs under the
// launcher's claim, each claim gone afterwards, attempts recorded in the ledger.
func TestLoopModeRunsNonInteractivelyAndReleasesClaims(t *testing.T) {
	f := newFixture(t, "l", "c", "s")
	writePlan(t, f.root, "plans/p/plan.md", "---\nstatus: In Progress\n---\n\n# GOAL\nparent\n")
	writePlan(t, f.root, "plans/p/01.md", "---\nstatus: Not Started\nsubtask_of: plans/p/plan.md\n---\n\n# GOAL\nchild\n")
	f.selecting("plans/p/plan.md")
	f.app.RunTests = func(layers ...string) []string { return nil }
	var held [][]string
	f.onAgent = func(context.Context) int {
		held = append(held, f.claimFiles())
		// the "agent" finishes whichever plan was last asked about
		last := f.agent[len(f.agent)-1].Args
		prompt := last[len(last)-1]
		for _, p := range []string{"plans/p/01.md", "plans/p/plan.md"} {
			if strings.Contains(prompt, "agent_docs/"+p) {
				planStatusSet(t, f.root, p, "Ready for Review")
			}
		}
		return 0
	}
	f.run()
	if len(f.agent) != 2 {
		t.Fatalf("agent runs %d\n%s", len(f.agent), f.out.String())
	}
	for i, c := range f.agent {
		if !contains(c.Args, "--print") {
			t.Errorf("run %d should be one-shot: %v", i, c.Args)
		}
		if len(held[i]) != 2 {
			t.Errorf("run %d: claims held during the run: %v", i, held[i])
		}
	}
	if left := f.claimFiles(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
	if got := len(agentsession.ReadLedger(repo.LedgerPath(f.root))); got != 2 {
		t.Errorf("ledger rows %d", got)
	}
	if !strings.Contains(f.out.String(), "[Loop Complete]") {
		t.Errorf("%s", f.out.String())
	}
}

func TestLoopModeRefusesWithoutTestRunner(t *testing.T) {
	f := newFixture(t, "l", "c", "s")
	writePlan(t, f.root, "plans/p/plan.md", "---\nstatus: In Progress\n---\n\n# GOAL\nparent\n")
	writePlan(t, f.root, "plans/p/01.md", "---\nstatus: Not Started\nsubtask_of: plans/p/plan.md\n---\n\n# GOAL\nchild\n")
	f.selecting("plans/p/plan.md")
	f.run()
	if len(f.agent) != 0 || !strings.Contains(f.out.String(), "Loop mode needs the automated test runner") {
		t.Errorf("%s", f.out.String())
	}
}

func planStatusSet(t *testing.T, root, rel, status string) {
	t.Helper()
	if err := planstatus.Set(root, rel, status, false); err != nil {
		t.Fatal(err)
	}
}
