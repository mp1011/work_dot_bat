// Package batch is the logic shared by loop mode and the auto daemon: the run-agent → run-tests →
// retry-or-halt loop, usage-limit handling, claim-release wrapping and the run summary. Every
// side effect is injected, so none of it needs a real agent or test run.
package batch

import (
	"fmt"
	"strings"
	"time"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/agentsession"
)

const MaxAttempts = 3

// AgentResult is what an agent run reports back. The zero value is "no verdict" (it ran);
// RateLimited means it bounced off a usage limit before getting to work.
type AgentResult struct {
	RateLimited bool
	ResetAt     *time.Time
}

type LoopDecision string

const (
	Passed    LoopDecision = "passed"
	Retry     LoopDecision = "retry"
	HardError LoopDecision = "hard-error"
)

// NextLoopDecision: no failures pass; otherwise retry until the attempt count reaches max.
func NextLoopDecision(failures []string, attempt, maxAttempts int) LoopDecision {
	switch {
	case len(failures) == 0:
		return Passed
	case attempt >= maxAttempts:
		return HardError
	}
	return Retry
}

// RateLimitStopMessage is the line printed when a whole batch stops on a usage-limit bounce.
func RateLimitStopMessage(planPath string, resetAt *time.Time) string {
	when := ""
	if resetAt != nil {
		when = " The limit should lift around " + resetAt.Local().Format("3:04:05 PM") + "."
	}
	return fmt.Sprintf("Agent usage limit hit — \"%s\" was not attempted, and the remaining tasks in the queue were not started.%s", planPath, when)
}

// PauseForRateLimit sits out a bounce until the reset time (plus a buffer), or a fixed back-off
// when the message gave no time, and returns how long it waited.
func PauseForRateLimit(resetAt *time.Time, now func() time.Time, sleep func(time.Duration)) time.Duration {
	d := agentsession.RateLimitPause(resetAt, now())
	sleep(d)
	return d
}

type RunOutcome string

const (
	OutcomePassed      RunOutcome = "passed"
	OutcomeHalted      RunOutcome = "halted"
	OutcomeNoChanges   RunOutcome = "no-changes"
	OutcomeRateLimited RunOutcome = "rate-limited"
)

type RunResult struct {
	Outcome  RunOutcome
	Attempts int
	Failures []string
	ResetAt  *time.Time // rate-limited only
}

// PlanRun configures RunPlanWithRetries.
type PlanRun struct {
	PlanPath string
	// RunAgent runs the agent on a prompt; attempt is 1-based.
	RunAgent func(prompt string, attempt int) AgentResult
	// RunTests returns the names of the failing checks (none = green).
	RunTests func() []string
	// ReadPlanContent is re-read for each follow-up prompt.
	ReadPlanContent func() string
	OnLog           func(string)
	MaxAttempts     int // 0 = MaxAttempts
	// HasCodeChanges guards "tests pass ⇒ the agent implemented it": an agent that halted
	// before touching a file leaves the tree green from the previous task, which says nothing
	// about this run. nil = always assume changes.
	HasCodeChanges     func() bool
	LauncherHoldsClaim bool
	OnTestFailure      func(failures []string, attempt int)
}

// RunPlanWithRetries is the `/do-work-yolo` → test → retry-or-halt loop.
//
// A usage-limit bounce is not an attempt: the agent never got to work, so running the tests and
// sending a follow-up would only burn the remaining attempts on instant bounces. The loop stops
// at once, `Attempts` counts only runs that really happened, and the caller pauses and resumes
// the plan later.
func RunPlanWithRetries(p PlanRun) RunResult {
	logf := p.OnLog
	if logf == nil {
		logf = func(string) {}
	}
	max := p.MaxAttempts
	if max == 0 {
		max = MaxAttempts
	}
	logf(fmt.Sprintf("Running /do-work-yolo (attempt 1 of %d)...", max))
	if r := p.RunAgent(agentcmd.BuildDoWorkYoloPrompt(p.PlanPath, p.LauncherHoldsClaim), 1); r.RateLimited {
		logf("The agent hit its usage limit — stopping without running tests or spending an attempt.")
		return RunResult{Outcome: OutcomeRateLimited, ResetAt: r.ResetAt}
	}
	for attempt := 1; ; attempt++ {
		logf("Running automated tests...")
		failures := p.RunTests()
		switch NextLoopDecision(failures, attempt, max) {
		case Passed:
			logf("All automated tests passed.")
			if p.HasCodeChanges != nil && !p.HasCodeChanges() {
				logf("...but the agent produced no code changes — recording this as a failed run, not a pass.")
				return RunResult{Outcome: OutcomeNoChanges, Attempts: attempt}
			}
			return RunResult{Outcome: OutcomePassed, Attempts: attempt}
		case HardError:
			if p.OnTestFailure != nil {
				p.OnTestFailure(failures, attempt)
			}
			return RunResult{Outcome: OutcomeHalted, Attempts: attempt, Failures: failures}
		}
		if p.OnTestFailure != nil {
			p.OnTestFailure(failures, attempt)
		}
		logf(fmt.Sprintf("Sending failures back to the agent (attempt %d of %d)...", attempt+1, max))
		content := ""
		if p.ReadPlanContent != nil {
			content = p.ReadPlanContent()
		}
		prompt := agentcmd.BuildFailureFollowupPrompt(p.PlanPath, content, failures, attempt, max)
		if r := p.RunAgent(prompt, attempt+1); r.RateLimited {
			logf(fmt.Sprintf("The agent hit its usage limit — stopping without spending attempt %d.", attempt+1))
			return RunResult{Outcome: OutcomeRateLimited, Attempts: attempt, ResetAt: r.ResetAt}
		}
	}
}

// WithClaimRelease runs work and then releases the plan's claim on every exit, a panic included.
// A launcher that spawns an agent which acquires its own claim cannot trust that agent to
// release it (it may be killed or simply never emit the release), so the guarantee that a clean
// exit leaves no claim behind lives here. Releasing an already-released claim is a no-op.
func WithClaimRelease[T any](planPath string, release func(planPath string), work func() T) T {
	defer release(planPath)
	return work()
}

// SummaryRow is one plan's final outcome in a batch.
type SummaryRow struct {
	Path     string
	Outcome  string
	Attempts int
}

var outcomeLabels = map[string]string{
	"ready-for-review": "Ready for Review",
	"halted":           "Halted (max attempts reached)",
	"rate-limited":     "Paused (agent usage limit)",
	"failed":           "Failed",
	"claimed":          "Claimed (never finished)",
}

func FormatSummary(rows []SummaryRow) string {
	if len(rows) == 0 {
		return "No tasks were processed."
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		label, ok := outcomeLabels[r.Outcome]
		if !ok {
			label = r.Outcome
		}
		lines[i] = fmt.Sprintf("- %s: %s (%d attempt(s))", r.Path, label, r.Attempts)
	}
	return strings.Join(lines, "\n")
}
