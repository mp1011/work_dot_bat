package batch

import (
	"fmt"
	"sort"
	"strings"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/planstatus"
)

// IsSubtaskEligible: anything not yet resolved.
func IsSubtaskEligible(t planstatus.Entry) bool {
	switch t.Status {
	case "Ready for Review", "Done", "Skipped", "Cancelled":
		return false
	}
	return true
}

// GetEligibleSubtasks are the parent's unresolved subtasks, by priority (P1 first, unprioritised
// last) then path.
func GetEligibleSubtasks(parent planstatus.Entry, tasks []planstatus.Entry) []planstatus.Entry {
	norm := planstatus.NormalizePlanPath(parent.Path)
	var subs []planstatus.Entry
	for _, t := range tasks {
		if t.SubtaskOf != "" && planstatus.NormalizePlanPath(t.SubtaskOf) == norm {
			subs = append(subs, t)
		}
	}
	rank := func(e planstatus.Entry) int {
		if e.Priority == 0 {
			return 1 << 30
		}
		return e.Priority
	}
	sort.SliceStable(subs, func(i, j int) bool {
		if rank(subs[i]) != rank(subs[j]) {
			return rank(subs[i]) < rank(subs[j])
		}
		return subs[i].Path < subs[j].Path
	})
	var out []planstatus.Entry
	for _, t := range subs {
		if IsSubtaskEligible(t) {
			out = append(out, t)
		}
	}
	return out
}

// LoopRun configures RunLoopUntilDone.
type LoopRun struct {
	Parent   planstatus.Entry
	GetTasks func() []planstatus.Entry
	// RunAgent runs the agent non-interactively; attempt is 1-based.
	RunAgent     func(prompt, planPath string, attempt int) AgentResult
	RunTests     func(layers ...string) []string
	AcquireClaim func(planPath string) bool // nil = no claims
	ReleaseClaim func(planPath string)
	ReadPlan     func(planPath string) string
	UpdateStatus func(path, status string) bool
	OnLog        func(string)
}

type LoopResult struct {
	Success  bool
	Reason   string // claim-blocked | rate-limited | status-not-ready | tests-failed
	Task     planstatus.Entry
	Failures []string
}

const maxTestAttempts = 3

func agentDocsPath(p string) string {
	if strings.HasPrefix(p, "agent_docs/") || strings.HasPrefix(p, `agent_docs\`) {
		return p
	}
	return "agent_docs/" + p
}

func find(tasks []planstatus.Entry, path string) (planstatus.Entry, bool) {
	norm := planstatus.NormalizePlanPath(path)
	for _, t := range tasks {
		if planstatus.NormalizePlanPath(t.Path) == norm {
			return t, true
		}
	}
	return planstatus.Entry{}, false
}

// RunLoopUntilDone works every eligible subtask of the parent in order, then the parent itself,
// each non-interactively under the launcher's claim: invoke the work skill, retry once if the
// plan is not Ready for Review, run the automated tests with up to two fix-ups, and stop at the
// first thing that needs a human. The claim is released on every exit from an iteration.
func RunLoopUntilDone(r LoopRun) LoopResult {
	logf := r.OnLog
	if logf == nil {
		logf = func(string) {}
	}
	ratelimited := func(t planstatus.Entry) LoopResult {
		logf("\n[Agent usage limit hit] Ending loop.")
		return LoopResult{Reason: "rate-limited", Task: t}
	}
	for {
		eligible := GetEligibleSubtasks(r.Parent, r.GetTasks())
		isParentRun := len(eligible) == 0
		target := r.Parent
		if !isParentRun {
			target = eligible[0]
		}
		logf("\n===================================================")
		if isParentRun {
			logf("[Loop Until Done] No eligible subtasks remain. Working on parent task: " + target.Path)
		} else {
			logf(fmt.Sprintf("[Loop Until Done] Next eligible subtask (%d remaining): %s", len(eligible), target.Path))
		}
		logf("===================================================")

		planPath := agentDocsPath(target.Path)
		if r.AcquireClaim != nil && !r.AcquireClaim(planPath) {
			logf(fmt.Sprintf("\n[MANUAL INTERVENTION REQUIRED] Could not acquire claim for \"%s\" (held by someone else). Ending loop.", planPath))
			return LoopResult{Reason: "claim-blocked", Task: target}
		}
		res, done := func() (LoopResult, bool) {
			if r.ReleaseClaim != nil {
				defer r.ReleaseClaim(planPath) // also on a panic
			}
			return runIteration(r, target, planPath, isParentRun, logf, ratelimited)
		}()
		if done {
			return res
		}
	}
}

// runIteration is one pass of the loop with the claim held. done is true when the loop ends
// (with res); false means carry on to the next task.
func runIteration(r LoopRun, target planstatus.Entry, planPath string, isParentRun bool, logf func(string),
	ratelimited func(planstatus.Entry) LoopResult) (res LoopResult, done bool) {
	if target.Status == "Not Started" || target.Status == "Suggestion" {
		logf(fmt.Sprintf("\nUpdating task status from \"%s\" to \"In Progress\"...", target.Status))
		if r.UpdateStatus(target.Path, "In Progress") {
			target.Status = "In Progress"
		}
	}

	logf("\nInvoking /do-work-yolo on " + planPath + "...")
	if a := r.RunAgent("/do-work-yolo "+planPath+agentcmd.LauncherClaimNote, planPath, 1); a.RateLimited {
		return ratelimited(target), true
	}

	statusNow := func() string {
		if t, ok := find(r.GetTasks(), target.Path); ok {
			return t.Status
		}
		return "Unknown"
	}
	current := statusNow()
	if current != "Ready for Review" {
		logf(fmt.Sprintf("\nTask \"%s\" is in status \"%s\" (not \"Ready for Review\"). Retrying with retry prompt (1 retry permitted)...", planPath, current))
		if a := r.RunAgent(agentcmd.BuildRetryPrompt(planPath, current, true), planPath, 2); a.RateLimited {
			return ratelimited(target), true
		}
		if current = statusNow(); current != "Ready for Review" {
			logf(fmt.Sprintf("\n[MANUAL INTERVENTION REQUIRED] Task \"%s\" is still not in \"Ready for Review\" status (current status: \"%s\") after retry. Ending loop.", planPath, current))
			return LoopResult{Reason: "status-not-ready", Task: target}, true
		}
	}

	runTier := func(tier, tierLabel string) (bool, LoopResult) {
		if r.RunTests == nil {
			return true, LoopResult{}
		}
		logf(fmt.Sprintf("\nRunning %s automated tests for \"%s\"...", tierLabel, planPath))
		failures := r.RunTests(tier)
		attempts := 1
		for len(failures) > 0 && attempts < maxTestAttempts {
			logf(fmt.Sprintf("\n%s automated tests failed (%s). Sending failures back to agent to fix (attempt %d of %d)...", tierLabel, strings.Join(failures, ", "), attempts+1, maxTestAttempts))
			content := ""
			if r.ReadPlan != nil {
				content = r.ReadPlan(planPath)
			}
			prompt := agentcmd.BuildFailureFollowupPrompt(planPath, content, failures, attempts, maxTestAttempts) + agentcmd.LauncherClaimNote
			if a := r.RunAgent(prompt, planPath, attempts+1); a.RateLimited {
				return false, ratelimited(target)
			}
			attempts++
			logf(fmt.Sprintf("\nRe-running %s automated tests (attempt %d of %d)...", tierLabel, attempts, maxTestAttempts))
			failures = r.RunTests(tier)
		}
		if len(failures) > 0 {
			logf(fmt.Sprintf("\n[MANUAL INTERVENTION REQUIRED] %s automated tests failed for \"%s\" after %d attempts (all retries exhausted). Failing checks: %s. Ending loop.", tierLabel, planPath, attempts, strings.Join(failures, ", ")))
			return false, LoopResult{Reason: "tests-failed", Task: target, Failures: failures}
		}
		logf(fmt.Sprintf("\n✅ %s automated tests passed for \"%s\".", tierLabel, planPath))
		return true, LoopResult{}
	}

	// 1. Run light tests after code changes
	if ok, res := runTier("light", "Light"); !ok {
		return res, true
	}

	// 2. Run heavy tests before marking ready for review
	if ok, res := runTier("heavy", "Heavy"); !ok {
		return res, true
	}

	if t, ok := find(r.GetTasks(), target.Path); !ok || t.Status != "Ready for Review" {
		r.UpdateStatus(target.Path, "Ready for Review")
	}
	if isParentRun {
		logf("\n===================================================")
		logf(fmt.Sprintf("✅ [Loop Complete] All subtasks and parent task \"%s\" have been completed!", r.Parent.Path))
		logf("===================================================")
		return LoopResult{Success: true, Task: r.Parent}, true
	}
	return LoopResult{}, false
}
