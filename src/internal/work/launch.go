package work

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/batch"
	"work_dot_bat/src/internal/repo"
)

// AgentRunner runs an agent command with the terminal passed straight through and returns its
// exit code. Cancelling ctx kills the agent.
type AgentRunner func(ctx context.Context, c agentcmd.Command, dir string) (int, error)

// ExecAgent is the real AgentRunner. The child gets this process's stdin/stdout/stderr
// unmodified (os.File handles, so no copying goroutine can swallow input). A Ctrl-C reaches the
// child through the shared terminal; this process ignores it for the child's lifetime so the
// deferred claim release still runs.
func ExecAgent(ctx context.Context, c agentcmd.Command, dir string) (int, error) {
	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code, nil
		}
		// Killed by a signal: a killed process never reported a real result, so not "passed".
		fmt.Fprintf(os.Stderr, "[ERROR] %s was terminated by a signal.\n", c.Cmd)
		return 1, nil
	}
	fmt.Fprintf(os.Stderr, "[ERROR] Failed to execute %s: %v\n", c.Cmd, err)
	return 1, nil
}

// GuardInterrupt makes Ctrl-C a no-op for this process until the returned stop is called, so a
// held claim is released by the normal deferred path instead of dying with the terminal's
// interrupt. Use it around any acquire…release span.
func GuardInterrupt() (stop func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	return func() { signal.Stop(sig) }
}

// AcquireLeafClaim takes the plan/branch claim as the launcher, recording this process's own
// pid so a claim leaked by a crash is pid-reclaimable instead of bound to the 4h TTL.
func (a App) AcquireLeafClaim(planPath string) bool {
	return a.claims().Run([]string{"acquire", planPath, "--owner", "work", "--kind", "launcher",
		"--pid", strconv.Itoa(a.ClaimEnv.PID)}) == 0
}

func (a App) ReleaseLeafClaim(planPath string) {
	a.claims().Run([]string{"release", planPath})
}

// WithClaim runs fn while holding the leaf plan's claim; the release is deferred, so it also
// runs when fn panics. held is false when the claim was refused (fn is not run).
func (a App) WithClaim(planPath string, fn func()) (held bool) {
	stop := a.guard()
	defer stop()
	if !a.AcquireLeafClaim(planPath) {
		return false
	}
	defer a.ReleaseLeafClaim(planPath)
	fn()
	return true
}

// session runs the agent once under a recorded ledger session and classifies how it ended.
// announce, if non-nil, is called after the session is recorded and before the agent starts.
func (a App) session(ctx context.Context, agent, planPath string, attempt *int, interactive bool, prompt string,
	announce func(name, sessionID, logFile string)) (agentsession.Verdict, error) {
	name := agentsession.FormatSessionName(planPath, a.now(), attempt)
	sessionID := agentsession.SessionID(name)
	logDir := repo.LogsDir(a.Root)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return agentsession.Verdict{}, err
	}
	logFile := filepath.Join(logDir, name+".log")
	ledger := repo.LedgerPath(a.Root)

	agentName := "claude"
	if agentcmd.IsAgy(agent) {
		agentName = "agy"
	}
	start := agentsession.Entry{"sessionName": name, "plan": planPath, "agent": agentName, "sessionId": sessionID}
	if agent == "a" {
		start["logFile"] = logFile
	}
	if _, err := agentsession.RecordStart(ledger, start, a.ClaimEnv.PID, a.now()); err != nil {
		return agentsession.Verdict{}, err
	}
	if announce != nil {
		announce(name, sessionID, logFile)
	}

	exitCode, status := 0, agentsession.Failed
	var verdict agentsession.Verdict
	defer func() {
		end := agentsession.Entry{"sessionName": name, "exitCode": exitCode, "status": string(status)}
		if verdict.ResetAt != nil {
			end["resetAt"] = agentsession.ISO(*verdict.ResetAt)
		}
		agentsession.RecordEnd(ledger, end, a.now())
	}()

	cmd := agentcmd.BuildCommand(agent, agentcmd.CommandOptions{
		RepoRoot: a.Root, Prompt: prompt, Interactive: interactive, SessionID: sessionID, LogFile: logFile,
	}, a.settings())
	code, err := a.runAgent()(ctx, cmd, a.Root)
	if err != nil {
		exitCode = 1
		return verdict, err
	}
	// The claude CLI exits 0 on a usage limit; the session transcript says which it was.
	exitCode = code
	verdict = agentsession.ClassifySession(agentName, sessionID, logFile, exitCode, a.Dirs, a.now())
	status = verdict.Status
	return verdict, nil
}

// LaunchInteractiveAgent starts the agent on the terminal for planPath with promptText,
// recording the session in the ledger and reporting how it ended.
func (a App) LaunchInteractiveAgent(ctx context.Context, agent, planPath, promptText string) error {
	verdict, err := a.session(ctx, agent, planPath, nil, true, promptText, func(name, sessionID, logFile string) {
		a.logf("\nSession Name: %s", name)
		if agent == "a" {
			a.logf("Log File:     %s", logFile)
			a.logf("\nLaunching Google Antigravity interactively with prompt:\n\"%s\"\n", promptText)
		} else {
			a.logf("Session ID:   %s", sessionID)
			a.logf("\nLaunching Claude Code interactively with prompt:\n\"%s\"\n", promptText)
		}
	})
	if err != nil {
		return err
	}
	if verdict.Status == agentsession.RateLimited {
		resets := ""
		if verdict.ResetAt != nil {
			resets = " — should lift around " + verdict.ResetAt.Local().Format("3:04:05 PM")
		}
		a.logf("\n[Agent session ended on a usage limit%s] Nothing was attempted; run it again once the limit lifts.", resets)
		return nil
	}
	a.logf("\n[Agent session completed]")
	return nil
}

// RunAgentNonInteractive runs the agent one-shot (--print) as an attempt of a loop run. An
// agent that bounced off a usage limit reports RateLimited; any other outcome is "it ran".
func (a App) RunAgentNonInteractive(ctx context.Context, agent, prompt, planPath string, attempt int) batch.AgentResult {
	verdict, err := a.session(ctx, agent, planPath, agentsession.Attempt(attempt), false, prompt, nil)
	if err != nil {
		a.logf("[ERROR] %v", err)
		return batch.AgentResult{}
	}
	return batch.AgentResult{RateLimited: verdict.Status == agentsession.RateLimited, ResetAt: verdict.ResetAt}
}

func (a App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
