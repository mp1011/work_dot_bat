package batch

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"work_dot_bat/src/internal/planstatus"
)

func TestNextLoopDecision(t *testing.T) {
	if NextLoopDecision(nil, 1, 3) != Passed {
		t.Error("no failures")
	}
	if NextLoopDecision([]string{"ESLint"}, 1, 3) != Retry || NextLoopDecision([]string{"ESLint"}, 2, 3) != Retry {
		t.Error("retry under max")
	}
	if NextLoopDecision([]string{"ESLint"}, 3, 3) != HardError {
		t.Error("hard error at max")
	}
	if MaxAttempts != 3 {
		t.Error("MaxAttempts")
	}
}

func TestFormatSummary(t *testing.T) {
	got := FormatSummary([]SummaryRow{
		{"plans/a.md", "ready-for-review", 1}, {"plans/b.md", "halted", 3},
		{"plans/c.md", "rate-limited", 0}, {"plans/d.md", "mystery", 2},
	})
	want := "- plans/a.md: Ready for Review (1 attempt(s))\n- plans/b.md: Halted (max attempts reached) (3 attempt(s))\n" +
		"- plans/c.md: Paused (agent usage limit) (0 attempt(s))\n- plans/d.md: mystery (2 attempt(s))"
	if got != want {
		t.Errorf("got:\n%s", got)
	}
	if !strings.Contains(FormatSummary(nil), "No tasks") {
		t.Error("empty")
	}
}

func TestRateLimitStopMessage(t *testing.T) {
	if got := RateLimitStopMessage("p.md", nil); got != `Agent usage limit hit — "p.md" was not attempted, and the remaining tasks in the queue were not started.` {
		t.Error(got)
	}
	r := time.Date(2026, 9, 18, 17, 30, 0, 0, time.UTC)
	got := RateLimitStopMessage("p.md", &r)
	if !strings.HasSuffix(got, " The limit should lift around "+r.Local().Format("3:04:05 PM")+".") {
		t.Error(got)
	}
}

func TestPauseForRateLimit(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	var slept time.Duration
	sleep := func(d time.Duration) { slept += d }
	reset := now.Add(10 * time.Minute)
	if d := PauseForRateLimit(&reset, clock, sleep); d != 11*time.Minute || slept != 11*time.Minute {
		t.Errorf("parsed reset: %v %v", d, slept)
	}
	slept = 0
	if d := PauseForRateLimit(nil, clock, sleep); d != 30*time.Minute || slept != 30*time.Minute {
		t.Errorf("fallback: %v %v", d, slept)
	}
}

func TestWithClaimRelease(t *testing.T) {
	var released []string
	rel := func(p string) { released = append(released, p) }
	if got := WithClaimRelease("a", rel, func() int { return 7 }); got != 7 || !reflect.DeepEqual(released, []string{"a"}) {
		t.Errorf("%v %v", got, released)
	}
	func() {
		defer func() { recover() }()
		WithClaimRelease("b", rel, func() int { panic("boom") })
	}()
	if !reflect.DeepEqual(released, []string{"a", "b"}) {
		t.Errorf("a panic must still release: %v", released)
	}
}

type harness struct {
	prompts  []string
	attempts []int
	runs     [][]string
	calls    int
	agent    func(prompt string, attempt int) AgentResult
}

func (h *harness) plan(testRuns ...[]string) PlanRun {
	return PlanRun{
		PlanPath: "agent_docs/plans/foo.md",
		RunAgent: func(prompt string, attempt int) AgentResult {
			h.prompts = append(h.prompts, prompt)
			h.attempts = append(h.attempts, attempt)
			if h.agent != nil {
				return h.agent(prompt, attempt)
			}
			return AgentResult{}
		},
		RunTests: func() []string {
			defer func() { h.calls++ }()
			if h.calls < len(testRuns) {
				return testRuns[h.calls]
			}
			return nil
		},
		ReadPlanContent: func() string { return "# GOAL\nfoo\n" },
	}
}

func TestRunPlanWithRetries(t *testing.T) {
	h := &harness{}
	got := RunPlanWithRetries(h.plan(nil))
	if got.Outcome != OutcomePassed || got.Attempts != 1 || len(got.Failures) != 0 || !reflect.DeepEqual(h.prompts, []string{"/do-work-yolo agent_docs/plans/foo.md"}) {
		t.Errorf("%+v %v", got, h.prompts)
	}

	// Fails twice then passes: three attempts, two follow-ups carrying the failing check.
	h = &harness{}
	got = RunPlanWithRetries(h.plan([]string{"ESLint"}, []string{"Unit Tests"}, nil))
	if got.Outcome != OutcomePassed || got.Attempts != 3 || len(h.prompts) != 3 {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(h.prompts[1], "ESLint") || !strings.Contains(h.prompts[1], "attempt 1 of 3") ||
		!strings.Contains(h.prompts[2], "Unit Tests") || !strings.Contains(h.prompts[2], "attempt 2 of 3") ||
		!strings.Contains(h.prompts[1], "# GOAL\nfoo") {
		t.Errorf("follow-ups: %q", h.prompts)
	}
	if !reflect.DeepEqual(h.attempts, []int{1, 2, 3}) {
		t.Errorf("attempt numbers %v", h.attempts)
	}

	// Always failing halts at MaxAttempts without asking the agent to fix it a fourth time.
	h = &harness{}
	got = RunPlanWithRetries(h.plan([]string{"ESLint"}, []string{"ESLint"}, []string{"ESLint"}))
	if got.Outcome != OutcomeHalted || got.Attempts != 3 || !reflect.DeepEqual(got.Failures, []string{"ESLint"}) || len(h.prompts) > MaxAttempts {
		t.Errorf("%+v prompts %d", got, len(h.prompts))
	}
}

func TestRunPlanNoChangesGuard(t *testing.T) {
	h := &harness{}
	p := h.plan(nil)
	p.HasCodeChanges = func() bool { return false }
	if got := RunPlanWithRetries(p); got.Outcome != OutcomeNoChanges || got.Attempts != 1 {
		t.Errorf("%+v", got)
	}
	h = &harness{}
	checked := 0
	p = h.plan([]string{"ESLint"}, nil)
	p.HasCodeChanges = func() bool { checked++; return true }
	if got := RunPlanWithRetries(p); got.Outcome != OutcomePassed || checked != 1 {
		t.Errorf("%+v checked %d: not consulted until tests pass", got, checked)
	}
}

func TestOnTestFailureOrderAndCoverage(t *testing.T) {
	var seq []string
	h := &harness{agent: func(_ string, attempt int) AgentResult {
		seq = append(seq, "agent-"+string(rune('0'+attempt)))
		return AgentResult{}
	}}
	p := h.plan([]string{"ESLint"}, nil)
	p.OnTestFailure = func(f []string, attempt int) { seq = append(seq, "test-failure") }
	RunPlanWithRetries(p)
	if !reflect.DeepEqual(seq, []string{"agent-1", "test-failure", "agent-2"}) {
		t.Errorf("%v", seq)
	}

	type rec struct {
		f []string
		a int
	}
	var recs []rec
	h = &harness{}
	p = h.plan([]string{"ESLint"}, []string{"ESLint"}, []string{"ESLint"})
	p.OnTestFailure = func(f []string, a int) { recs = append(recs, rec{f, a}) }
	RunPlanWithRetries(p)
	if len(recs) != 3 || recs[0].a != 1 || recs[2].a != 3 {
		t.Errorf("every failed attempt incl. the last should report: %v", recs)
	}
}

func TestRunPlanUsageLimitBounce(t *testing.T) {
	reset := time.Date(2026, 9, 18, 17, 30, 0, 0, time.UTC)
	limit := AgentResult{RateLimited: true, ResetAt: &reset}

	// Bounce on attempt 1: no tests run, no attempt spent.
	h := &harness{agent: func(string, int) AgentResult { return limit }}
	got := RunPlanWithRetries(h.plan([]string{"never"}))
	if got.Outcome != OutcomeRateLimited || got.Attempts != 0 || got.ResetAt == nil || !got.ResetAt.Equal(reset) || h.calls != 0 {
		t.Errorf("%+v tests run %d", got, h.calls)
	}

	// Bounce on the follow-up: only the attempt that really happened is counted.
	h = &harness{agent: func(_ string, attempt int) AgentResult {
		if attempt == 2 {
			return limit
		}
		return AgentResult{}
	}}
	got = RunPlanWithRetries(h.plan([]string{"ESLint"}, nil))
	if got.Outcome != OutcomeRateLimited || got.Attempts != 1 || h.calls != 1 {
		t.Errorf("%+v tests run %d", got, h.calls)
	}
}

func entries(es ...planstatus.Entry) []planstatus.Entry { return es }

func TestEligibleSubtasksOrderAndFiltering(t *testing.T) {
	parent := planstatus.Entry{Path: "plans/test/plan.md"}
	tasks := entries(
		planstatus.Entry{Path: "plans/test/plan.md", Status: "In Progress"},
		planstatus.Entry{Path: "plans/test/03.md", Status: "Not Started", SubtaskOf: "plans/test/plan.md"},
		planstatus.Entry{Path: "plans/test/02.md", Status: "In Progress", SubtaskOf: "plans/test/plan.md", Priority: 2},
		planstatus.Entry{Path: "plans/test/01.md", Status: "Not Started", SubtaskOf: "plans/test/plan.md", Priority: 1},
		planstatus.Entry{Path: "plans/test/04.md", Status: "Ready for Review", SubtaskOf: "plans/test/plan.md", Priority: 1},
		planstatus.Entry{Path: "plans/test/05.md", Status: "Cancelled", SubtaskOf: "plans/test/plan.md"},
		planstatus.Entry{Path: "plans/test/06.md", Status: "Skipped", SubtaskOf: "plans/test/plan.md"},
		planstatus.Entry{Path: "plans/other/01.md", Status: "Not Started", SubtaskOf: "plans/other/plan.md"},
	)
	var got []string
	for _, s := range GetEligibleSubtasks(parent, tasks) {
		got = append(got, s.Path)
	}
	if !reflect.DeepEqual(got, []string{"plans/test/01.md", "plans/test/02.md", "plans/test/03.md"}) {
		t.Errorf("%v", got)
	}
	// agent_docs/ prefixed parent path matches too.
	if n := len(GetEligibleSubtasks(planstatus.Entry{Path: "agent_docs/plans/test/plan.md"}, tasks)); n != 3 {
		t.Errorf("prefix not normalised: %d", n)
	}
	if IsSubtaskEligible(planstatus.Entry{Status: "Done"}) || !IsSubtaskEligible(planstatus.Entry{Status: "Blocked - User Input Required"}) {
		t.Error("eligibility")
	}
}

// loopHarness simulates a plan set that the fake agent mutates.
type loopHarness struct {
	t        *testing.T
	tasks    []planstatus.Entry
	acquired []string
	released []string
	invoked  []string
	prompts  []string
	logs     []string
	// agent decides what the fake agent does to the task set for (planPath, attempt, callIndex).
	agent func(h *loopHarness, planPath string, attempt int) AgentResult
	tests      [][]string
	calls      int
	layerCalls []string
}

func (h *loopHarness) set(path, status string) {
	for i := range h.tasks {
		if strings.HasSuffix(path, h.tasks[i].Path) {
			h.tasks[i].Status = status
		}
	}
}

func (h *loopHarness) run(parent planstatus.Entry) LoopResult {
	return RunLoopUntilDone(LoopRun{
		Parent:   parent,
		GetTasks: func() []planstatus.Entry { return append([]planstatus.Entry(nil), h.tasks...) },
		RunAgent: func(prompt, planPath string, attempt int) AgentResult {
			h.invoked = append(h.invoked, planPath)
			h.prompts = append(h.prompts, prompt)
			if h.agent != nil {
				return h.agent(h, planPath, attempt)
			}
			h.set(planPath, "Ready for Review")
			return AgentResult{}
		},
		RunTests: func(layers ...string) []string {
			defer func() { h.calls++ }()
			if len(layers) > 0 {
				h.layerCalls = append(h.layerCalls, layers[0])
			}
			if h.calls < len(h.tests) {
				return h.tests[h.calls]
			}
			return nil
		},
		AcquireClaim: func(p string) bool { h.acquired = append(h.acquired, p); return true },
		ReleaseClaim: func(p string) { h.released = append(h.released, p) },
		ReadPlan:     func(string) string { return "# Test Plan" },
		UpdateStatus: func(p, s string) bool { h.set(p, s); return true },
		OnLog:        func(m string) { h.logs = append(h.logs, m) },
	})
}

func threeTaskSet() (planstatus.Entry, []planstatus.Entry) {
	parent := planstatus.Entry{Path: "plans/test/plan.md", Status: "In Progress"}
	return parent, entries(parent,
		planstatus.Entry{Path: "plans/test/01.md", Status: "Not Started", SubtaskOf: "plans/test/plan.md"},
		planstatus.Entry{Path: "plans/test/02.md", Status: "Not Started", SubtaskOf: "plans/test/plan.md"},
	)
}

func TestLoopRunsSubtasksThenParentAndReleasesEachClaim(t *testing.T) {
	parent, tasks := threeTaskSet()
	h := &loopHarness{t: t, tasks: tasks}
	res := h.run(parent)
	want := []string{"agent_docs/plans/test/01.md", "agent_docs/plans/test/02.md", "agent_docs/plans/test/plan.md"}
	if !res.Success || !reflect.DeepEqual(h.acquired, want) || !reflect.DeepEqual(h.released, want) || !reflect.DeepEqual(h.invoked, want) {
		t.Errorf("%+v\nacquired %v\nreleased %v\ninvoked %v", res, h.acquired, h.released, h.invoked)
	}
	if !strings.HasPrefix(h.prompts[0], "/do-work-yolo agent_docs/plans/test/01.md\n\nNote: work.bat already holds the plan/branch claim") {
		t.Errorf("prompt %q", h.prompts[0])
	}
}

func TestLoopRetriesOnceWhenNotReadyForReview(t *testing.T) {
	parent, tasks := threeTaskSet()
	tasks = tasks[:2]
	h := &loopHarness{t: t, tasks: tasks}
	calls := 0
	h.agent = func(h *loopHarness, planPath string, attempt int) AgentResult {
		calls++
		switch {
		case strings.HasSuffix(planPath, "01.md") && calls == 1:
			h.set(planPath, "In Progress")
		default:
			h.set(planPath, "Ready for Review")
		}
		return AgentResult{}
	}
	res := h.run(parent)
	if !res.Success || calls != 3 {
		t.Fatalf("%+v calls %d", res, calls)
	}
	if !strings.Contains(h.prompts[1], `ended with status "In Progress" instead of "Ready for Review"`) || !strings.Contains(h.prompts[1], "work.bat already holds") {
		t.Errorf("retry prompt %q", h.prompts[1])
	}

	// Still not ready after the retry: stop for a human, claim released.
	parent, tasks = threeTaskSet()
	h = &loopHarness{t: t, tasks: tasks[:2]}
	h.agent = func(h *loopHarness, planPath string, attempt int) AgentResult { return AgentResult{} }
	res = h.run(parent)
	if res.Success || res.Reason != "status-not-ready" || res.Task.Path != "plans/test/01.md" || len(h.invoked) != 2 ||
		!reflect.DeepEqual(h.released, []string{"agent_docs/plans/test/01.md"}) {
		t.Errorf("%+v invoked %v released %v", res, h.invoked, h.released)
	}
}

func TestLoopTestRetryBudget(t *testing.T) {
	parent := planstatus.Entry{Path: "plans/p/plan.md", Status: "In Progress"}
	tasks := entries(parent, planstatus.Entry{Path: "plans/p/01.md", Status: "Not Started", SubtaskOf: "plans/p/plan.md"})

	// Pass on the third test attempt: two fix-up prompts, then continue.
	h := &loopHarness{t: t, tasks: append([]planstatus.Entry(nil), tasks...), tests: [][]string{{"lint"}, {"unit"}, nil, nil}}
	res := h.run(parent)
	if !res.Success {
		t.Fatalf("%+v", res)
	}
	var fixups []string
	for _, p := range h.prompts {
		if strings.HasPrefix(p, "Automated tests failed") {
			fixups = append(fixups, p)
		}
	}
	if len(fixups) != 2 || !strings.Contains(fixups[0], "attempt 1 of 3") || !strings.Contains(fixups[1], "attempt 2 of 3") ||
		!strings.HasSuffix(fixups[0], "still run the `concurrent-check` read.") || !strings.Contains(fixups[0], "# Test Plan") {
		t.Errorf("fix-ups %q", fixups)
	}

	// All three fail: stop with the failing checks, claim released.
	h = &loopHarness{t: t, tasks: append([]planstatus.Entry(nil), tasks...), tests: [][]string{{"lint"}, {"lint"}, {"lint", "unit"}}}
	res = h.run(parent)
	if res.Success || res.Reason != "tests-failed" || !reflect.DeepEqual(res.Failures, []string{"lint", "unit"}) || len(h.released) != 1 {
		t.Errorf("%+v released %v", res, h.released)
	}
	if !strings.Contains(strings.Join(h.logs, "\n"), "after 3 attempts (all retries exhausted). Failing checks: lint, unit. Ending loop.") {
		t.Errorf("%v", h.logs)
	}

	// The budget resets for the next task in the loop.
	parent, tasks = threeTaskSet()
	h = &loopHarness{t: t, tasks: tasks, tests: [][]string{{"x"}, {"x"}, nil, {"y"}, {"y"}, nil, nil}}
	if res := h.run(parent); !res.Success {
		t.Errorf("budget should reset per task: %+v", res)
	}
}

func TestLoopStopsOnUsageLimitAndOnBlockedClaim(t *testing.T) {
	parent, tasks := threeTaskSet()
	h := &loopHarness{t: t, tasks: tasks}
	h.agent = func(*loopHarness, string, int) AgentResult { return AgentResult{RateLimited: true} }
	res := h.run(parent)
	if res.Success || res.Reason != "rate-limited" || len(h.released) != 1 {
		t.Errorf("%+v released %v", res, h.released)
	}

	parent, tasks = threeTaskSet()
	h = &loopHarness{t: t, tasks: tasks}
	got := RunLoopUntilDone(LoopRun{
		Parent: parent, GetTasks: func() []planstatus.Entry { return tasks },
		AcquireClaim: func(string) bool { return false },
		RunAgent:     func(string, string, int) AgentResult { t.Error("agent must not run"); return AgentResult{} },
	})
	if got.Success || got.Reason != "claim-blocked" {
		t.Errorf("%+v", got)
	}
}

func TestLoopReleasesClaimWhenAnIterationPanics(t *testing.T) {
	parent, tasks := threeTaskSet()
	h := &loopHarness{t: t, tasks: tasks}
	h.agent = func(*loopHarness, string, int) AgentResult { panic("boom") }
	func() {
		defer func() { recover() }()
		h.run(parent)
	}()
	if len(h.released) != 1 {
		t.Errorf("released %v", h.released)
	}
}

func TestLoopRunsLightTestsAfterIterationAndHeavyTestsBeforeReadyForReview(t *testing.T) {
	parent := planstatus.Entry{Path: "plans/test/single.md", Status: "In Progress"}
	h := &loopHarness{t: t, tasks: []planstatus.Entry{parent}}
	res := h.run(parent)
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	wantLayers := []string{"light", "heavy"}
	if !reflect.DeepEqual(h.layerCalls, wantLayers) {
		t.Errorf("got layer calls %v, want %v", h.layerCalls, wantLayers)
	}
}

func TestLoopFailingLightTestPreventsProceeding(t *testing.T) {
	parent := planstatus.Entry{Path: "plans/test/single.md", Status: "In Progress"}
	h := &loopHarness{
		t:     t,
		tasks: []planstatus.Entry{parent},
		tests: [][]string{{"lint"}, {"lint"}, {"lint"}}, // all 3 attempts fail
	}
	res := h.run(parent)
	if res.Success || res.Reason != "tests-failed" {
		t.Fatalf("expected tests-failed, got %+v", res)
	}
	// All calls should have been for "light", never proceeding to "heavy"
	for _, l := range h.layerCalls {
		if l != "light" {
			t.Errorf("expected only light test calls on failure, got %v", h.layerCalls)
			break
		}
	}
}
