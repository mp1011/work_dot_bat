package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"work_dot_bat/src/internal/proc"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/worktree"
)

const (
	MaxAttempts    = 3
	AgentHeartbeat = 5 * time.Minute
	AgentTimeout   = 60 * time.Minute
)

// GitCode runs git in dir and reports the exit code and output (a non-zero exit is not an error:
// `git diff --cached --quiet` uses it to answer).
type GitCode func(dir string, args ...string) (code int, stdout, stderr string)

func RealGitCode(dir string, args ...string) (int, string, string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return 0, out.String(), errb.String()
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), out.String(), errb.String()
	}
	return 1, out.String(), err.Error()
}

// StepRun is what one agent run produced.
type StepRun struct {
	Output      string // the agent's final message
	RateLimited bool
	ResetAt     *time.Time
	RawText     string // the text that carried the limit message
	TimedOut    bool
}

// StepMeta describes one agent run to a RunAgent seam.
type StepMeta struct {
	Attempt   int
	Phase     string
	Worktree  string
	SessionID string
	LogFile   string
}

// Runner runs plans. Every external effect is a field so tests can replace it; New wires the
// real ones.
type Runner struct {
	Root        string
	ProjectName string
	Agent       string // an agentcmd definition key, id, or alias (built-ins: "a" Antigravity, "c" Claude)
	DaemonDoc   string
	MaxAttempts int // 0 = MaxAttempts
	Heartbeat   time.Duration
	Timeout     time.Duration

	Log      func(string)
	Now      func() time.Time
	Sleep    func(ctx context.Context, d time.Duration) error
	ClaimEnv planclaims.Env
	Dirs     agentsession.Dirs
	Ledger   string

	Settings func() agentcmd.Settings
	// RunAgent, if set, replaces the real agent process (tests).
	RunAgent func(ctx context.Context, prompt string, meta StepMeta) StepRun
	// RunProc runs a process; defaults to RunProcess.
	RunProc func(ctx context.Context, name string, args []string, o proc.Options) (int, error)

	Git            GitCode
	GitRead        GitFunc
	EnsureWorktree func(branch string) (worktree.EnsureResult, error)
	Bootstrap      func(path string) bool
	StartDevServer func(path string)
	SetStatus      func(treeRoot, planPath, status string) error
	// RunTests runs the project's automated checks in cwd and returns the failing check names.
	RunTests  func(ctx context.Context, cwd string, layers ...string) []string
	ReadPlans func(root string) []Plan
	// OnActiveClaim is told which claim is held so a signal handler can release it.
	OnActiveClaim func(*ClaimRecord)
}

// ClaimRecord is the plan and branch claims a run holds.
type ClaimRecord struct {
	Root   string
	Scopes []planclaims.Scope
	env    planclaims.Env
}

// Release drops every scope; releasing what is not held is a no-op.
func (c *ClaimRecord) Release() {
	if c == nil {
		return
	}
	for _, s := range c.Scopes {
		_ = planclaims.Release(c.Root, s.Scope, s.ID)
	}
}

func (c *ClaimRecord) renew() {
	for _, s := range c.Scopes {
		_, _ = c.env.Renew(c.Root, s.Scope, s.ID)
	}
}

func (r Runner) log(format string, a ...any) {
	if r.Log != nil {
		r.Log(fmt.Sprintf(format, a...))
	}
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r Runner) maxAttempts() int {
	if r.MaxAttempts > 0 {
		return r.MaxAttempts
	}
	return MaxAttempts
}

func (r Runner) git() GitCode {
	if r.Git != nil {
		return r.Git
	}
	return RealGitCode
}

func (r Runner) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r Runner) settings() agentcmd.Settings {
	if r.Settings != nil {
		return r.Settings()
	}
	return agentcmd.DefaultSettings()
}

// agentDef is the definition of the selected agent.
func (r Runner) agentDef() agentcmd.Definition { return r.settings().Agent(r.Agent) }

func (r Runner) readPlans(root string) []Plan {
	if r.ReadPlans != nil {
		return r.ReadPlans(root)
	}
	return ReadCheckoutPlans(root)
}

func (r Runner) setStatus(tree, planPath, status string) error {
	if r.SetStatus != nil {
		return r.SetStatus(tree, planPath, status)
	}
	return planstatus.Set(tree, planPath, status, false)
}

func isPlanBookkeeping(file string) bool {
	return strings.HasPrefix(file, "agent_docs/plans/") || file == "agent_docs/plan_status.js" ||
		strings.HasPrefix(file, repo.StateDirName+"/")
}

// IsWorktreeDirty: changes other than plan bookkeeping and the tool's own state directory.
// An unreadable tree reads as clean.
func IsWorktreeDirty(git GitCode, worktreePath string) bool {
	code, out, _ := git(worktreePath, "status", "--porcelain")
	if code != 0 {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		file := strings.ReplaceAll(strings.TrimSpace(line[min(2, len(line)):]), `\`, "/")
		if !isPlanBookkeeping(file) {
			return true
		}
	}
	return false
}

// HasNonPlanChanges: whether the worktree differs from baseSHA (or has untracked files) outside
// plan bookkeeping — i.e. the agent actually changed something.
func HasNonPlanChanges(git GitCode, worktreePath, baseSHA string) bool {
	changed := map[string]bool{}
	if code, out, _ := git(worktreePath, "diff", "--name-only", baseSHA); code == 0 {
		for _, l := range strings.Split(out, "\n") {
			if l = strings.ReplaceAll(strings.TrimSpace(l), `\`, "/"); l != "" {
				changed[l] = true
			}
		}
	}
	if code, out, _ := git(worktreePath, "status", "--porcelain"); code == 0 {
		for _, l := range strings.Split(out, "\n") {
			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "??") {
				if f := strings.ReplaceAll(strings.TrimSpace(l[2:]), `\`, "/"); f != "" {
					changed[f] = true
				}
			}
		}
	}
	for f := range changed {
		if !isPlanBookkeeping(f) {
			return true
		}
	}
	return false
}

var (
	blockedAnyRE = regexp.MustCompile(`(?is)##\s*Blocked[^\r\n]*\r?\n(.*?)(?:\r?\n##\s|$)`)
	goalSectRE   = regexp.MustCompile(`(?is)#+\s*GOAL[^\r\n]*\r?\n(.*?)(?:\r?\n#+\s|$)`)
)

// WriteBlockedReason replaces (or appends) the plan's "## Blocked — Needs User Input" section.
func WriteBlockedReason(planFile, reason string) error {
	b, _ := os.ReadFile(planFile)
	content := string(b)
	section := "## Blocked — Needs User Input\n" + strings.TrimSpace(reason) + "\n"
	if blockedAnyRE.MatchString(content) {
		content = blockedAnyRE.ReplaceAllLiteralString(content, section)
	} else {
		content = strings.TrimRight(content, " \t\r\n") + "\n\n" + section
	}
	return os.WriteFile(planFile, []byte(content), 0o644)
}

// SubtaskValidation is the verdict on a breakdown.
type SubtaskValidation struct {
	Valid    bool
	Reason   string
	Subtasks []string
}

// ValidateSubtasks checks that the breakdown the agent wrote is usable: at least one subtask
// points at the parent, each has a non-empty GOAL and is registered (has a status).
func ValidateSubtasks(worktreePath string, parent Plan) SubtaskValidation {
	parentNorm := norm(parent.Path)
	type sub struct{ path, content string }
	var subs []sub
	for _, rel := range planstatus.WalkPlanFiles(worktreePath) {
		b, err := os.ReadFile(filepath.Join(worktreePath, "agent_docs", filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		fm := planstatus.ParseFrontmatter(string(b))
		if fm.SubtaskOf != "" && norm(fm.SubtaskOf) == parentNorm {
			subs = append(subs, sub{rel, string(b)})
		}
	}
	paths := make([]string, len(subs))
	for i, s := range subs {
		paths[i] = s.path
	}
	if len(subs) == 0 {
		return SubtaskValidation{Reason: "No subtask files found with subtask_of pointing to " + parent.Path}
	}
	for _, s := range subs {
		if m := goalSectRE.FindStringSubmatch(s.content); m == nil || strings.TrimSpace(m[1]) == "" {
			return SubtaskValidation{Reason: "Subtask " + s.path + " has an empty or missing GOAL section", Subtasks: paths}
		}
		if planstatus.ParseFrontmatter(s.content).Status == nil {
			return SubtaskValidation{Reason: "Subtask " + s.path + " is not registered (missing frontmatter status)", Subtasks: paths}
		}
	}
	return SubtaskValidation{Valid: true, Subtasks: paths}
}

// CommitResult is the outcome of committing in a worktree.
type CommitResult struct {
	OK    bool
	Empty bool
	Error string
}

// CommitInWorktree stages everything and commits it; nothing staged is OK and Empty.
func CommitInWorktree(git GitCode, worktreePath, msg string) CommitResult {
	fail := func(code int, out, errOut, fallback string) CommitResult {
		e := errOut
		if e == "" {
			e = out
		}
		if e == "" {
			e = fallback
		}
		return CommitResult{Error: strings.TrimSpace(e)}
	}
	if code, out, e := git(worktreePath, "add", "-A"); code != 0 {
		return fail(code, out, e, "git add -A failed")
	}
	if code, _, _ := git(worktreePath, "diff", "--cached", "--quiet"); code == 0 {
		return CommitResult{OK: true, Empty: true}
	}
	if code, out, e := git(worktreePath, "commit", "-m", msg); code != 0 {
		return fail(code, out, e, "git commit failed")
	}
	return CommitResult{OK: true}
}

// FormatUsageLimitMessage is the line printed while waiting out a usage limit.
func FormatUsageLimitMessage(planPath, rawText string, resetAt *time.Time) string {
	plan := strings.TrimPrefix(planPath, "agent_docs/")
	m := resetFullRE.FindStringSubmatch(rawText)
	if m != nil && resetAt != nil {
		resume := resetAt.Add(time.Minute)
		str := resume.Local().Format("3:04:05 PM")
		if loc, err := time.LoadLocation(m[2]); err == nil {
			str = strings.ToLower(resume.In(loc).Format("3:04PM"))
		}
		return fmt.Sprintf("⏸  Usage limit hit — %s. Waiting until %s, then continuing %s.", m[1], str, plan)
	}
	if resetAt != nil {
		return fmt.Sprintf("⏸  Usage limit hit. Waiting until %s, then continuing %s.", resetAt.Add(time.Minute).Local().Format("3:04:05 PM"), plan)
	}
	return fmt.Sprintf("⏸  Usage limit hit — reset time unparseable. Waiting 30m, then continuing %s.", plan)
}

var resetFullRE = regexp.MustCompile(`(?i)(resets\s+\d{1,2}(?::\d{2})?\s*(?:am|pm)\s*\(([^)]+)\))`)

// StuckBlock is printed when an agent produced no result before its timeout.
func StuckBlock(planPath string, timeout time.Duration, label string) string {
	return strings.Join([]string{
		"",
		"===================================================",
		fmt.Sprintf("AGENT STUCK: %s", strings.TrimPrefix(planPath, "agent_docs/")),
		fmt.Sprintf("No result after %dm — killed the \"%s\" process.", int(timeout.Round(time.Minute)/time.Minute), label),
		"This usually means the agent CLI itself is wedged (e.g. an",
		"auth/login failure it silently retries forever) rather than",
		"the plan being genuinely hard to finish.",
		fmt.Sprintf("Check that \"%s\" is logged in and reachable, then restart the daemon.", label),
		"===================================================",
		"",
	}, "\n")
}

type step struct {
	entry    Plan
	phase    string
	attempt  int
	problem  Problem
	tree     string
	planFull string
	claim    *ClaimRecord
	parent   string // assess-parent target (agent_docs-relative)
}

type stepResult struct {
	output   string
	timedOut bool
}

// execStep runs one agent step (work or an assess phase), waiting out usage limits. It returns a
// timed-out result, or ctx's error when cancelled.
func (r Runner) execStep(ctx context.Context, s step) (stepResult, error) {
	interrupted := false
	for {
		dirty := IsWorktreeDirty(r.git(), s.tree)
		var prompt string
		switch s.phase {
		case "work":
			switch {
			case interrupted && dirty:
				prompt = BuildDaemonContinuationPrompt(s.planFull, r.DaemonDoc, s.problem, s.attempt, r.maxAttempts(), "limit")
			case s.attempt == 1 && dirty:
				prompt = BuildDaemonContinuationPrompt(s.planFull, r.DaemonDoc, s.problem, s.attempt, r.maxAttempts(), "interrupted")
			case s.attempt == 1:
				prompt = BuildDaemonWorkPrompt(s.planFull, r.DaemonDoc)
			default:
				prompt = BuildDaemonFollowupPrompt(s.planFull, s.problem, s.attempt, r.maxAttempts())
			}
		default:
			target := s.planFull
			if s.phase == "assess-parent" && s.parent != "" {
				target = "agent_docs/" + s.parent
			}
			prompt = BuildAssessPrompt(target)
		}

		attempt := s.attempt
		name := agentsession.FormatSessionName(s.planFull, r.now(), &attempt)
		sessionID := agentsession.SessionID(name)
		logDir := repo.LogsDir(r.Root)
		_ = os.MkdirAll(logDir, 0o755)
		logFile := filepath.Join(logDir, name+".log")
		def := r.agentDef()
		label := def.ID
		start := agentsession.Entry{"sessionName": name, "plan": s.planFull, "agent": label, "reader": def.Session.Reader, "sessionId": sessionID}
		if def.Session.LogFile {
			start["logFile"] = logFile
		}
		pid := os.Getpid()
		_, _ = agentsession.RecordStart(r.Ledger, start, pid, r.now())
		end := func(code int, status string) {
			_, _ = agentsession.RecordEnd(r.Ledger, agentsession.Entry{"sessionName": name, "exitCode": code, "status": status}, r.now())
		}

		run := r.runStep(ctx, prompt, StepMeta{Attempt: s.attempt, Phase: s.phase, Worktree: s.tree, SessionID: sessionID, LogFile: logFile}, s, name, label)
		if run.TimedOut {
			end(1, "timed-out")
			r.log("%s", StuckBlock(s.entry.Path, r.timeout(), label))
			return stepResult{timedOut: true}, nil
		}
		if err := ctx.Err(); err != nil {
			end(1, "failed")
			return stepResult{}, err
		}
		if run.RateLimited {
			end(0, "rate-limited")
			now := r.now()
			r.log("%s", FormatUsageLimitMessage(s.entry.Path, run.RawText, run.ResetAt))
			if err := r.sleep(ctx, agentsession.RateLimitPause(run.ResetAt, now)); err != nil {
				return stepResult{}, err
			}
			interrupted = true
			continue
		}
		end(0, "completed")
		return stepResult{output: run.Output}, nil
	}
}

func (r Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return AgentTimeout
}

func (r Runner) heartbeat() time.Duration {
	if r.Heartbeat > 0 {
		return r.Heartbeat
	}
	return AgentHeartbeat
}

// runStep runs the agent once and reads back its final message and how it ended.
func (r Runner) runStep(ctx context.Context, prompt string, meta StepMeta, s step, sessionName, label string) StepRun {
	if r.RunAgent != nil {
		run := r.RunAgent(ctx, prompt, meta)
		if !run.RateLimited {
			v := agentsession.ClassifyExit(0, "", run.Output, r.now())
			if v.Status == agentsession.RateLimited {
				run.RateLimited, run.ResetAt, run.RawText = true, v.ResetAt, run.Output
			}
		} else if run.RawText == "" {
			run.RawText = run.Output
		}
		return run
	}
	settings := r.settings()
	def := settings.Agent(r.Agent)
	dirs := def.Dirs(r.Dirs)
	cmd := agentcmd.BuildInvocation(r.Agent, s.tree, prompt, agentcmd.InvocationOptions{
		SessionName: sessionName, SessionID: meta.SessionID, LogFile: meta.LogFile,
	}, settings)
	runProc := r.RunProc
	if runProc == nil {
		runProc = proc.Run
	}
	code, err := runProc(ctx, cmd.Cmd, cmd.Args, proc.Options{
		Dir: s.tree, Timeout: r.timeout(), Heartbeat: r.heartbeat(),
		OnHeartbeat: func(elapsed time.Duration) {
			r.log("still waiting on %s for %s (%dm elapsed, no result yet)", label, s.entry.Path, int(elapsed.Round(time.Minute)/time.Minute))
			s.claim.renew()
		},
	})
	if errors.Is(err, proc.ErrTimedOut) {
		return StepRun{TimedOut: true}
	}
	if err != nil {
		r.log("[ERROR] %v", err)
	}
	out := ReadFinalMessage(def.Session.Reader, meta.SessionID, meta.LogFile, dirs, r.Root)
	run := StepRun{Output: out}
	if v := agentsession.ClassifySession(def.Session.Reader, meta.SessionID, meta.LogFile, code, dirs, r.now()); v.Status == agentsession.RateLimited {
		run.RateLimited, run.ResetAt, run.RawText = true, v.ResetAt, out
	}
	return run
}

// PlanResult is how working one plan ended.
type PlanResult struct {
	Status   string // ready-for-review | blocked | subtasks | timed-out | skipped | failed | cancelled
	Reason   string
	Subtasks []string
	Attempts int
}

func (r Runner) block(tree, planPath, reason, commitSuffix string) {
	_ = WriteBlockedReason(filepath.Join(tree, "agent_docs", filepath.FromSlash(planPath)), reason)
	_ = r.setStatus(tree, planPath, "Blocked - User Input Required")
	CommitInWorktree(r.git(), tree, fmt.Sprintf("auto: %s (blocked: %s)", planPath, commitSuffix))
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func problemSummary(p Problem) string {
	switch {
	case len(p.Failures) > 0:
		return "Tests failed: " + strings.Join(p.Failures, ", ")
	case len(p.Gaps) > 0:
		return "Assessment gaps: " + strings.Join(p.Gaps, "; ")
	case p.NoChanges:
		return "no-changes"
	case p.Message != "":
		return p.Message
	}
	return "Attempts exhausted"
}

// RunPlan works one plan: claim it, ensure its worktree, then loop agent → outcome → tests →
// diff check → /assess-done, retrying with a follow-up prompt up to MaxAttempts. The claim is
// released on every exit.
func (r Runner) RunPlan(ctx context.Context, entry Plan, plans []Plan) PlanResult {
	branch := DaemonBranchFor(entry.Path, plans)
	host, _ := os.Hostname()
	owner := planclaims.Owner{Kind: "daemon", Label: r.Agent, PID: os.Getpid(), Host: host}
	scopes := []planclaims.Scope{
		{Scope: "plan", ID: entry.Path, Branch: branch},
		{Scope: "branch", ID: branch, Branch: branch},
	}
	acq, err := r.ClaimEnv.Acquire(r.Root, scopes, owner, planclaims.DaemonTTL, false)
	if err != nil || !acq.OK {
		why := acq.BlockedScope
		if acq.Decision.Reason != "" {
			why = acq.Decision.Reason
		}
		if err != nil {
			why = err.Error()
		}
		r.log("Skipping %s: claim held (%s)", entry.Path, why)
		return PlanResult{Status: "skipped", Reason: "claim held"}
	}
	claim := &ClaimRecord{Root: r.Root, Scopes: scopes, env: r.ClaimEnv}
	if r.OnActiveClaim != nil {
		r.OnActiveClaim(claim)
		defer r.OnActiveClaim(nil)
	}
	defer claim.Release()

	ensure := r.EnsureWorktree
	if ensure == nil {
		ensure = func(b string) (worktree.EnsureResult, error) {
			return worktree.Ensure(r.Root, b, r.ProjectName, worktree.RunCommand, func(m string) { r.log("%s", m) })
		}
	}
	wt, err := ensure(branch)
	if err != nil || !wt.OK {
		reason := wt.Reason
		if err != nil {
			reason = err.Error()
		}
		r.log("Failed to ensure worktree for %s: %s", branch, reason)
		return PlanResult{Status: "failed", Reason: reason}
	}
	if wt.Created {
		if r.Bootstrap != nil && !r.Bootstrap(wt.Path) {
			r.log("Worktree bootstrap failed: %s", wt.Path)
			return PlanResult{Status: "failed", Reason: "bootstrap failed"}
		}
		if r.StartDevServer != nil {
			r.StartDevServer(wt.Path)
		}
	}
	_ = r.setStatus(wt.Path, entry.Path, "In Progress")

	_, sha, _ := r.git()(wt.Path, "rev-parse", "HEAD")
	baseSHA := strings.TrimSpace(sha)
	planFull := "agent_docs/" + entry.Path
	max := r.maxAttempts()
	var problem Problem

	for attempt := 1; attempt <= max; attempt++ {
		r.log("\n--- Plan: %s | Branch: %s | Worktree: %s | Attempt %d/%d ---", entry.Path, branch, wt.Path, attempt, max)
		res, err := r.execStep(ctx, step{entry: entry, phase: "work", attempt: attempt, problem: problem, tree: wt.Path, planFull: planFull, claim: claim})
		if err != nil {
			return PlanResult{Status: "cancelled", Reason: err.Error()}
		}
		if res.timedOut {
			return PlanResult{Status: "timed-out"}
		}
		outcome := ParseOutcome(res.output)
		if outcome == nil {
			r.log("Agent outcome: ❌ (missing outcome marker)")
			problem = Problem{Message: "Missing outcome marker in final assistant message. You must end with AUTO_OUTCOME: done | subtasks | blocked — <reason>."}
			continue
		}
		r.log("Agent outcome: %s ✅", outcome.Kind)

		switch outcome.Kind {
		case "blocked":
			reason := outcome.Reason
			if reason == "" {
				reason = "Blocked by agent request"
			}
			r.log("Plan blocked: %s", reason)
			r.block(wt.Path, entry.Path, reason, reason)
			return PlanResult{Status: "blocked", Reason: reason}

		case "subtasks":
			v := ValidateSubtasks(wt.Path, entry)
			if !v.Valid {
				r.log("Subtasks validation: ❌ (%s)", v.Reason)
				problem = Problem{Message: "Subtask breakdown validation failed: " + v.Reason}
				continue
			}
			r.log("Subtasks validation: ✅ (%d subtasks)", len(v.Subtasks))
			if c := CommitInWorktree(r.git(), wt.Path, fmt.Sprintf("auto: %s (broke into subtasks)", entry.Path)); !c.OK {
				reason := "git commit failed: " + c.Error
				r.log("Commit failed: ❌ (%s)", reason)
				r.block(wt.Path, entry.Path, reason, "commit failure")
				return PlanResult{Status: "blocked", Reason: reason}
			}
			return PlanResult{Status: "subtasks", Subtasks: v.Subtasks}

		case "done":
			if r.RunTests == nil {
				return PlanResult{Status: "failed", Reason: "no automated test runner configured"}
			}
			if failures := r.RunTests(ctx, wt.Path, "light"); len(failures) > 0 {
				r.log("Light tests: ❌ (%s)", strings.Join(failures, ", "))
				problem = Problem{Failures: failures}
				continue
			}
			r.log("Light tests: ✅")
			if !HasNonPlanChanges(r.git(), wt.Path, baseSHA) {
				r.log("Diff: ❌ (no non-plan changes produced)")
				problem = Problem{NoChanges: true}
				continue
			}
			r.log("Diff: ✅")

			ares, err := r.execStep(ctx, step{entry: entry, phase: "assess", attempt: attempt, tree: wt.Path, planFull: planFull, claim: claim})
			if err != nil {
				return PlanResult{Status: "cancelled", Reason: err.Error()}
			}
			if ares.timedOut {
				return PlanResult{Status: "timed-out"}
			}
			if a := ParseAssessment(ares.output); a == nil || !a.Complete {
				gaps := []string{"Assessment reported requirements incomplete"}
				if a != nil && len(a.Gaps) > 0 {
					gaps = a.Gaps
				}
				r.log("Assessment: ❌ (%s)", strings.Join(gaps, "; "))
				problem = Problem{Gaps: gaps}
				continue
			}
			r.log("Assessment: ✅ (complete)")

			if failures := r.RunTests(ctx, wt.Path, "heavy"); len(failures) > 0 {
				r.log("Heavy tests: ❌ (%s)", strings.Join(failures, ", "))
				problem = Problem{Failures: failures}
				continue
			}
			r.log("Heavy tests: ✅")

			_ = r.setStatus(wt.Path, entry.Path, "Ready for Review")
			if c := CommitInWorktree(r.git(), wt.Path, "auto: "+entry.Path); !c.OK {
				reason := "git commit failed: " + c.Error
				r.log("Commit failed: ❌ (%s)", reason)
				r.block(wt.Path, entry.Path, reason, "commit failure")
				return PlanResult{Status: "blocked", Reason: reason}
			}
			r.log("Commit: ✅ (auto: %s)", entry.Path)
			r.closeOutParents(ctx, entry, wt.Path, claim)
			return PlanResult{Status: "ready-for-review", Attempts: attempt}
		}
	}
	reason := problemSummary(problem)
	r.log("\nPlan exhausted attempts (max %d): %s", max, reason)
	r.block(wt.Path, entry.Path, reason, "attempts exhausted")
	return PlanResult{Status: "blocked", Reason: reason, Attempts: max}
}

// closeOutParents walks up the subtask chain: once every subtask of a parent is resolved, run
// /assess-done on the parent and mark it Done, or Blocked with the gaps.
func (r Runner) closeOutParents(ctx context.Context, entry Plan, tree string, claim *ClaimRecord) {
	cur := entry
	for cur.SubtaskOf != "" {
		parentPath := norm(cur.SubtaskOf)
		plans := r.readPlans(tree)
		parent := find(plans, parentPath)
		if parent == nil {
			break
		}
		var subs []Plan
		for _, p := range plans {
			if p.SubtaskOf != "" && norm(p.SubtaskOf) == parentPath {
				subs = append(subs, p)
			}
		}
		all := len(subs) > 0
		for _, s := range subs {
			if s.Status != "Ready for Review" && s.Status != "Done" && s.Status != "Cancelled" {
				all = false
			}
		}
		if !all {
			break
		}
		r.log("All subtasks resolved for parent %s. Running /assess-done on parent...", parent.Path)
		res, err := r.execStep(ctx, step{entry: *parent, phase: "assess-parent", attempt: 1, tree: tree,
			planFull: "agent_docs/" + parent.Path, parent: parent.Path, claim: claim})
		if err != nil || res.timedOut {
			break
		}
		a := ParseAssessment(res.output)
		if a == nil {
			break
		}
		if a.Complete {
			r.log("Parent close-out: %s assessed complete", parent.Path)
			if err := r.setStatus(tree, parent.Path, "Done"); err != nil {
				// plan-status refuses Done while a subtask is only Ready for Review: the human
				// confirms those first. Say so rather than report a close-out that did not happen.
				r.log("Parent close-out: %s could not be marked Done (%s)", parent.Path, firstLine(err.Error()))
				break
			}
			if c := CommitInWorktree(r.git(), tree, fmt.Sprintf("auto: %s (done)", parent.Path)); !c.OK {
				r.log("Failed to commit parent completion: %s", c.Error)
				break
			}
			r.log("Parent close-out: %s is Done ✅", parent.Path)
			cur = *parent
			cur.Status = "Done"
			continue
		}
		gaps := a.Gaps
		if len(gaps) == 0 {
			gaps = []string{"Assessment reported requirements incomplete"}
		}
		r.log("Parent close-out: %s is Blocked ❌", parent.Path)
		bullets := make([]string, len(gaps))
		for i, g := range gaps {
			bullets[i] = "- " + g
		}
		_ = WriteBlockedReason(filepath.Join(tree, "agent_docs", filepath.FromSlash(parent.Path)), strings.Join(bullets, "\n"))
		_ = r.setStatus(tree, parent.Path, "Blocked - User Input Required")
		CommitInWorktree(r.git(), tree, fmt.Sprintf("auto: %s (blocked: incomplete requirements)", parent.Path))
		break
	}
}
