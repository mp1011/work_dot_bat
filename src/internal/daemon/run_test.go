package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"work_dot_bat/src/internal/proc"

	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/repo"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

type rig struct {
	t      *testing.T
	root   string
	wt     string
	r      Runner
	logs   []string
	calls  []string // "<phase>:<attempt>"
	prompt []string
	// agent decides the reply: phase ("work"/"assess"/"assess-parent"), attempt, wt path.
	agent func(phase string, attempt int, wt string) StepRun
	tests      [][]string
	ntest      int
	testLayers []string
	slept      []time.Duration
}

func newRig(t *testing.T) *rig {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for k, v := range map[string]string{"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t"} {
		t.Setenv(k, v)
	}
	base := t.TempDir()
	root := filepath.ToSlash(filepath.Join(base, "proj"))
	os.MkdirAll(filepath.Join(root, "agent_docs", "plans"), 0o755)
	os.WriteFile(filepath.Join(root, "agent_docs", "plans", "x.md"), []byte("---\nstatus: Not Started\nauto: true\n---\n\n# GOAL\nDo x\n"), 0o644)
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "config", "core.autocrlf", "false")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-q", "-m", "init")

	g := &rig{t: t, root: root}
	g.wt = filepath.ToSlash(filepath.Join(base, "proj-"+planstatus.SlugForPlan("plans/x.md")))
	g.r = Runner{
		Root: root, ProjectName: "proj", Agent: "c", DaemonDoc: "doc.md",
		Log:      func(m string) { g.logs = append(g.logs, m) },
		ClaimEnv: planclaims.DefaultEnv(),
		Dirs:     agentsession.Dirs{Projects: t.TempDir(), AgyBrain: t.TempDir()},
		Ledger:   repo.LedgerPath(root),
		Sleep:    func(_ context.Context, d time.Duration) error { g.slept = append(g.slept, d); return nil },
		RunAgent: func(ctx context.Context, prompt string, m StepMeta) StepRun {
			g.calls = append(g.calls, m.Phase+":"+itoa(m.Attempt))
			g.prompt = append(g.prompt, prompt)
			return g.agent(m.Phase, m.Attempt, m.Worktree)
		},
		RunTests: func(_ context.Context, _ string, layers ...string) []string {
			defer func() { g.ntest++ }()
			if len(layers) > 0 {
				g.testLayers = append(g.testLayers, layers[0])
			}
			if g.ntest < len(g.tests) {
				return g.tests[g.ntest]
			}
			return nil
		},
	}
	return g
}

func (g *rig) plan() Plan {
	for _, p := range ReadCheckoutPlans(g.root) {
		if p.Path == "plans/x.md" {
			return p
		}
	}
	g.t.Fatal("plan missing")
	return Plan{}
}

func (g *rig) run() PlanResult {
	p := g.plan()
	return g.r.RunPlan(context.Background(), p, []Plan{p})
}

func (g *rig) claims() []string {
	entries, _ := os.ReadDir(repo.ClaimsDir(g.root))
	var n []string
	for _, e := range entries {
		n = append(n, e.Name())
	}
	return n
}

func (g *rig) wtStatus(rel string) string {
	b, _ := os.ReadFile(filepath.Join(g.wt, "agent_docs", filepath.FromSlash(rel)))
	fm := planstatus.ParseFrontmatter(string(b))
	if fm.Status == nil {
		return ""
	}
	return *fm.Status
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(content), 0o644)
}

func TestDoneOutcomeEndsReadyForReviewAndCommits(t *testing.T) {
	g := newRig(t)
	g.agent = func(phase string, attempt int, wt string) StepRun {
		if phase == "work" {
			writeFile(t, wt, "src/feature.txt", "feature\n")
			return StepRun{Output: "implemented\nAUTO_OUTCOME: done"}
		}
		return StepRun{Output: "all met\nASSESSMENT: complete"}
	}
	res := g.run()
	if res.Status != "ready-for-review" || res.Attempts != 1 {
		t.Fatalf("%+v\n%s", res, strings.Join(g.logs, "\n"))
	}
	if g.wtStatus("plans/x.md") != "Ready for Review" {
		t.Errorf("status %q", g.wtStatus("plans/x.md"))
	}
	if got := gitIn(t, g.wt, "log", "-1", "--format=%s"); got != "auto: plans/x.md" {
		t.Errorf("commit %q", got)
	}
	if gitIn(t, g.wt, "status", "--porcelain") != "" {
		t.Error("worktree should be clean after the commit")
	}
	if strings.Join(g.calls, ",") != "work:1,assess:1" {
		t.Errorf("calls %v", g.calls)
	}
	if !strings.HasPrefix(g.prompt[0], "/do-work-yolo agent_docs/plans/x.md\n\nDAEMON MODE") || g.prompt[1] != "/assess-done agent_docs/plans/x.md" {
		t.Errorf("prompts %q", g.prompt)
	}
	if left := g.claims(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
	if ledger := agentsession.ReadLedger(repo.LedgerPath(g.root)); len(ledger) != 2 || ledger[0]["status"] != "completed" {
		t.Errorf("ledger %v", ledger)
	}
	// main is untouched: the work happened on the branch.
	if gitIn(t, g.root, "log", "-1", "--format=%s") != "init" {
		t.Error("main moved")
	}
}

func TestClaimIsHeldDuringTheRun(t *testing.T) {
	g := newRig(t)
	var during []string
	g.agent = func(phase string, attempt int, wt string) StepRun {
		during = g.claims()
		return StepRun{Output: "AUTO_OUTCOME: blocked — stop"}
	}
	g.run()
	if len(during) != 2 {
		t.Errorf("plan and branch claims expected during the run, got %v", during)
	}
	if len(g.claims()) != 0 {
		t.Error("claims must be released")
	}
}

func TestBlockedOutcomeWritesReasonAndStatus(t *testing.T) {
	g := newRig(t)
	g.agent = func(string, int, string) StepRun {
		return StepRun{Output: "AUTO_OUTCOME: blocked — need the production API key"}
	}
	res := g.run()
	if res.Status != "blocked" || res.Reason != "need the production API key" {
		t.Fatalf("%+v", res)
	}
	if g.wtStatus("plans/x.md") != "Blocked - User Input Required" {
		t.Errorf("status %q", g.wtStatus("plans/x.md"))
	}
	b, _ := os.ReadFile(filepath.Join(g.wt, "agent_docs/plans/x.md"))
	if !strings.Contains(string(b), "## Blocked — Needs User Input\nneed the production API key\n") {
		t.Errorf("plan:\n%s", b)
	}
	if got := gitIn(t, g.wt, "log", "-1", "--format=%s"); got != "auto: plans/x.md (blocked: need the production API key)" {
		t.Errorf("commit %q", got)
	}
	if ExtractBlockedReason(strings.SplitN(string(b), "---\n\n", 2)[1]) != "need the production API key" {
		t.Error("reason should round-trip for the daemon loop")
	}
}

func TestSubtasksOutcomeValidatesAndCommits(t *testing.T) {
	g := newRig(t)
	g.agent = func(phase string, attempt int, wt string) StepRun {
		writeFile(t, wt, "agent_docs/plans/x/01.md", "---\nstatus: Not Started\nsubtask_of: plans/x.md\n---\n\n# GOAL\nfirst part\n")
		return StepRun{Output: "AUTO_OUTCOME: subtasks"}
	}
	res := g.run()
	if res.Status != "subtasks" || len(res.Subtasks) != 1 || res.Subtasks[0] != "plans/x/01.md" {
		t.Fatalf("%+v\n%s", res, strings.Join(g.logs, "\n"))
	}
	if got := gitIn(t, g.wt, "log", "-1", "--format=%s"); got != "auto: plans/x.md (broke into subtasks)" {
		t.Errorf("commit %q", got)
	}
	// The parent stays In Progress.
	if g.wtStatus("plans/x.md") != "In Progress" {
		t.Errorf("parent status %q", g.wtStatus("plans/x.md"))
	}

	// An unregistered or goal-less subtask is sent back, not accepted.
	g2 := newRig(t)
	g2.agent = func(phase string, attempt int, wt string) StepRun {
		writeFile(t, wt, "agent_docs/plans/x/01.md", "---\nstatus: Not Started\nsubtask_of: plans/x.md\n---\n\n# GOAL\n\n")
		return StepRun{Output: "AUTO_OUTCOME: subtasks"}
	}
	res = g2.run()
	if res.Status != "blocked" || !strings.Contains(res.Reason, "empty or missing GOAL") || len(g2.calls) != 3 {
		t.Errorf("%+v calls %v", res, g2.calls)
	}
	if !strings.Contains(g2.prompt[1], "Problem: Subtask breakdown validation failed:") {
		t.Errorf("follow-up %q", g2.prompt[1])
	}
}

func TestMissingMarkerRetriesThenBlocks(t *testing.T) {
	g := newRig(t)
	g.agent = func(string, int, string) StepRun { return StepRun{Output: "I did some stuff."} }
	res := g.run()
	if res.Status != "blocked" || res.Attempts != 3 || len(g.calls) != 3 {
		t.Fatalf("%+v calls %v", res, g.calls)
	}
	if !strings.Contains(res.Reason, "Missing outcome marker") || !strings.Contains(g.prompt[1], "Follow-up on agent_docs/plans/x.md (attempt 2 of 3)") {
		t.Errorf("%q / %q", res.Reason, g.prompt[1])
	}
	if g.wtStatus("plans/x.md") != "Blocked - User Input Required" {
		t.Errorf("status %q", g.wtStatus("plans/x.md"))
	}
}

func TestFailingTestsAreSentBackWithTheFailures(t *testing.T) {
	g := newRig(t)
	g.tests = [][]string{{"lint"}, nil}
	g.agent = func(phase string, attempt int, wt string) StepRun {
		if phase == "work" {
			writeFile(t, wt, "code.txt", "x")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	res := g.run()
	if res.Status != "ready-for-review" || res.Attempts != 2 || strings.Join(g.calls, ",") != "work:1,work:2,assess:2" {
		t.Fatalf("%+v calls %v", res, g.calls)
	}
	if !strings.Contains(g.prompt[1], "Automated test/check failures:\n- lint") {
		t.Errorf("%q", g.prompt[1])
	}
}

func TestNoCodeChangesIsNotAPass(t *testing.T) {
	g := newRig(t)
	g.agent = func(phase string, attempt int, wt string) StepRun { return StepRun{Output: "AUTO_OUTCOME: done"} }
	res := g.run()
	if res.Status != "blocked" || res.Reason != "no-changes" || g.ntest != 3 {
		t.Errorf("%+v tests %d", res, g.ntest)
	}
	if !strings.Contains(g.prompt[1], "completed without producing any code changes") {
		t.Errorf("%q", g.prompt[1])
	}
}

func TestIncompleteAssessmentSendsGapsBack(t *testing.T) {
	g := newRig(t)
	var assessed int
	g.agent = func(phase string, attempt int, wt string) StepRun {
		if phase == "work" {
			writeFile(t, wt, "code.txt", "x")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		assessed++
		if assessed == 1 {
			return StepRun{Output: "- tests missing\n\nASSESSMENT: incomplete"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	res := g.run()
	if res.Status != "ready-for-review" || res.Attempts != 2 {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(g.prompt[2], "Assessment identified incomplete requirements (gaps):\n- tests missing") {
		t.Errorf("%q", g.prompt[2])
	}
}

func TestUsageLimitPausesThenContinues(t *testing.T) {
	g := newRig(t)
	reset := time.Now().Add(10 * time.Minute)
	n := 0
	g.agent = func(phase string, attempt int, wt string) StepRun {
		n++
		if n == 1 {
			writeFile(t, wt, "partial.txt", "half done") // uncommitted work from before the bounce
			return StepRun{RateLimited: true, ResetAt: &reset, RawText: "You've hit your session limit · resets 3pm (UTC)"}
		}
		if phase == "work" {
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	g.r.Now = func() time.Time { return reset.Add(-10 * time.Minute) }
	res := g.run()
	if res.Status != "ready-for-review" || res.Attempts != 1 {
		t.Fatalf("%+v\n%s", res, strings.Join(g.logs, "\n"))
	}
	if len(g.slept) != 1 || g.slept[0] != 11*time.Minute {
		t.Errorf("paused %v, want the time to the reset plus a minute", g.slept)
	}
	if !strings.Contains(strings.Join(g.logs, "\n"), "⏸  Usage limit hit — resets 3pm (UTC). Waiting until") {
		t.Errorf("logs:\n%s", strings.Join(g.logs, "\n"))
	}
	// The retry carries the continuation wording because the worktree holds the agent's own work.
	if !strings.Contains(g.prompt[1], "You were interrupted by a usage limit; the uncommitted changes in this worktree are your own earlier work") {
		t.Errorf("%q", g.prompt[1])
	}
	// A bounce is not an attempt and both runs are in the ledger.
	var statuses []any
	for _, e := range agentsession.ReadLedger(repo.LedgerPath(g.root)) {
		statuses = append(statuses, e["status"])
	}
	if len(statuses) < 3 || statuses[0] != "rate-limited" {
		t.Errorf("ledger statuses %v", statuses)
	}
}

func TestCancellationReleasesTheClaim(t *testing.T) {
	g := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	g.r.RunAgent = func(ctx context.Context, prompt string, m StepMeta) StepRun {
		cancel()
		<-ctx.Done()
		return StepRun{}
	}
	p := g.plan()
	res := g.r.RunPlan(ctx, p, []Plan{p})
	if res.Status != "cancelled" {
		t.Errorf("%+v", res)
	}
	if left := g.claims(); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
}

func TestHeldClaimSkipsThePlan(t *testing.T) {
	g := newRig(t)
	g.agent = func(string, int, string) StepRun { t.Error("agent must not run"); return StepRun{} }
	other := planclaims.CLI{Root: g.root, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}, Env: planclaims.DefaultEnv()}
	if other.Run([]string{"acquire", "plans/x.md", "--owner", "someone", "--kind", "daemon", "--pid", itoa(os.Getppid())}) != 0 {
		t.Fatal("setup")
	}
	if res := g.run(); res.Status != "skipped" || res.Reason != "claim held" {
		t.Errorf("%+v", res)
	}
	if len(g.claims()) != 2 {
		t.Errorf("the other holder's claims must stay: %v", g.claims())
	}
}

func TestStuckAgentStopsTheDaemonRun(t *testing.T) {
	g := newRig(t)
	g.r.RunAgent = nil
	g.r.Timeout = 300 * time.Millisecond
	g.r.RunProc = func(ctx context.Context, name string, args []string, o proc.Options) (int, error) {
		<-time.After(o.Timeout)
		return 1, proc.ErrTimedOut
	}
	res := g.run()
	if res.Status != "timed-out" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(strings.Join(g.logs, "\n"), "AGENT STUCK: plans/x.md") {
		t.Errorf("logs:\n%s", strings.Join(g.logs, "\n"))
	}
	if ledger := agentsession.ReadLedger(repo.LedgerPath(g.root)); len(ledger) != 1 || ledger[0]["status"] != "timed-out" {
		t.Errorf("ledger %v", ledger)
	}
	if len(g.claims()) != 0 {
		t.Error("claims must be released")
	}
}

func TestParentCloseOutAssessesAndReportsRefusedDone(t *testing.T) {
	g := newRig(t)
	// Make x.md a parent with a single subtask 01 (auto via the parent).
	writeFile(t, g.root, "agent_docs/plans/x/01.md", "---\nstatus: Not Started\nsubtask_of: plans/x.md\n---\n\n# GOAL\nonly part\n")
	gitIn(t, g.root, "add", ".")
	gitIn(t, g.root, "commit", "-q", "-m", "subtask")
	plans := ReadCheckoutPlans(g.root)
	var sub Plan
	for _, p := range plans {
		if p.Path == "plans/x/01.md" {
			sub = p
		}
	}
	g.agent = func(phase string, attempt int, wt string) StepRun {
		switch phase {
		case "work":
			writeFile(t, wt, "code.txt", "x")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	res := g.r.RunPlan(context.Background(), sub, plans)
	if res.Status != "ready-for-review" {
		t.Fatalf("%+v\n%s", res, strings.Join(g.logs, "\n"))
	}
	if strings.Join(g.calls, ",") != "work:1,assess:1,assess-parent:1" {
		t.Errorf("calls %v", g.calls)
	}
	// plan-status refuses Done for a parent whose subtask is only Ready for Review (the human
	// confirms the subtask first), and the close-out says so instead of claiming success.
	if g.wtStatus("plans/x.md") == "Done" || !strings.Contains(strings.Join(g.logs, "\n"), "Parent close-out: plans/x.md could not be marked Done") {
		t.Errorf("parent %q\n%s", g.wtStatus("plans/x.md"), strings.Join(g.logs, "\n"))
	}
}

func TestParentCloseOutIncompleteBlocksParent(t *testing.T) {
	g := newRig(t)
	writeFile(t, g.root, "agent_docs/plans/x/01.md", "---\nstatus: Not Started\nsubtask_of: plans/x.md\n---\n\n# GOAL\nonly part\n")
	gitIn(t, g.root, "add", ".")
	gitIn(t, g.root, "commit", "-q", "-m", "subtask")
	plans := ReadCheckoutPlans(g.root)
	var sub Plan
	for _, p := range plans {
		if p.Path == "plans/x/01.md" {
			sub = p
		}
	}
	g.agent = func(phase string, attempt int, wt string) StepRun {
		switch phase {
		case "work":
			writeFile(t, wt, "code.txt", "x")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		case "assess":
			return StepRun{Output: "ASSESSMENT: complete"}
		}
		return StepRun{Output: "- the parent goal is bigger\n\nASSESSMENT: incomplete"}
	}
	g.r.RunPlan(context.Background(), sub, plans)
	if g.wtStatus("plans/x.md") != "Blocked - User Input Required" {
		t.Errorf("parent %q", g.wtStatus("plans/x.md"))
	}
	b, _ := os.ReadFile(filepath.Join(g.wt, "agent_docs/plans/x.md"))
	if !strings.Contains(string(b), "## Blocked — Needs User Input\n- the parent goal is bigger\n") {
		t.Errorf("plan:\n%s", b)
	}
}

func TestLoopWorksFirstEligiblePlanThenIdles(t *testing.T) {
	g := newRig(t)
	g.agent = func(phase string, attempt int, wt string) StepRun {
		if phase == "work" {
			writeFile(t, wt, "code.txt", "x")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	var out strings.Builder
	l := Loop{Runner: g.r, Out: &out, MaxIterations: 2, PollInterval: time.Millisecond}
	l.Runner.Sleep = func(context.Context, time.Duration) error { return nil }
	if code := l.Run(context.Background()); code != 0 {
		t.Fatalf("exit %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "Next eligible plan: plans/x.md (Not Started)") {
		t.Errorf("%s", s)
	}
	// Second pass: the plan is now Ready for Review in its worktree, so nothing is eligible.
	if !strings.Contains(s, "Idle — no eligible plans. Waiting...") {
		t.Errorf("%s", s)
	}
}

func TestLoopReportsBlockedAndStops(t *testing.T) {
	g := newRig(t)
	g.agent = func(string, int, string) StepRun { return StepRun{Output: "AUTO_OUTCOME: blocked — need a key"} }
	var out strings.Builder
	opened := 0
	l := Loop{Runner: g.r, Out: &out, MaxIterations: 3, PollInterval: time.Millisecond,
		OpenBlocked: func(string) { opened++ }}
	l.Runner.Sleep = func(context.Context, time.Duration) error { return nil }
	l.Run(context.Background())
	s := out.String()
	for _, want := range []string{"BLOCKED — daemon will not proceed (1):", "  Plan:     plans/x.md", "  Reason:   need a key",
		"Resolve the blockers in the worktree, then change status to resume."} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
	if opened != 1 {
		t.Errorf("the editor should open once per blocked plan, opened %d times", opened)
	}
}

func TestLoopReturns1WhenAnAgentIsStuck(t *testing.T) {
	g := newRig(t)
	g.r.RunAgent = nil
	g.r.Timeout = 100 * time.Millisecond
	g.r.RunProc = func(ctx context.Context, name string, args []string, o proc.Options) (int, error) {
		return 1, proc.ErrTimedOut
	}
	var out strings.Builder
	l := Loop{Runner: g.r, Out: &out, PollInterval: time.Millisecond}
	if code := l.Run(context.Background()); code != 1 {
		t.Errorf("exit %d", code)
	}
}

func TestLoopStopsOnCancel(t *testing.T) {
	g := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	var out strings.Builder
	l := Loop{Runner: g.r, Out: &out, PollInterval: time.Hour}
	l.Runner.Sleep = nil
	l.ReadPlans = func() []Plan { cancel(); return nil }
	if code := l.Run(ctx); code != 0 {
		t.Errorf("exit %d", code)
	}
}

func TestStatusReportsAutoPlansReadOnly(t *testing.T) {
	g := newRig(t)
	var out strings.Builder
	before := gitIn(t, g.root, "status", "--porcelain")
	if code := Status(Config{Root: g.root, Out: &out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "ELIGIBLE (next first):") || !strings.Contains(out.String(), "plans/x.md") || !strings.Contains(out.String(), "[main]") {
		t.Errorf("%s", out.String())
	}
	if gitIn(t, g.root, "status", "--porcelain") != before {
		t.Error("auto_status must not change anything")
	}
}

func TestDaemonRunsLightAfterIterationAndHeavyBeforeReadyForReview(t *testing.T) {
	g := newRig(t)
	g.agent = func(phase string, attempt int, wt string) StepRun {
		if phase == "work" {
			writeFile(t, wt, "code.txt", "hello")
			return StepRun{Output: "AUTO_OUTCOME: done"}
		}
		return StepRun{Output: "ASSESSMENT: complete"}
	}
	res := g.run()
	if res.Status != "ready-for-review" {
		t.Fatalf("expected ready-for-review, got %+v", res)
	}
	wantLayers := []string{"light", "heavy"}
	if !reflect.DeepEqual(g.testLayers, wantLayers) {
		t.Errorf("got test layers %v, want %v", g.testLayers, wantLayers)
	}
}

func TestDaemonFailingLightTestPreventsProceeding(t *testing.T) {
	g := newRig(t)
	// Light tests fail for all 3 attempts
	g.tests = [][]string{{"lint"}, {"lint"}, {"lint"}}
	g.agent = func(phase string, attempt int, wt string) StepRun {
		writeFile(t, wt, "code.txt", "hello")
		return StepRun{Output: "AUTO_OUTCOME: done"}
	}
	res := g.run()
	if res.Status != "blocked" {
		t.Fatalf("expected blocked, got %+v", res)
	}
	// Verify that heavy was never called, only light
	for _, l := range g.testLayers {
		if l != "light" {
			t.Errorf("expected only light test executions on failure, got %v", g.testLayers)
			break
		}
	}
}
