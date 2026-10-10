package work

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"work_dot_bat/internal/agentcmd"
)

// BuildPreparePushPrompt is the agent's prepare-for-push invocation. With skipMechanical it tells
// the agent the lint/typecheck/test steps (3-6) already ran, so it goes straight to the
// agentic-only ones: doc-sync, suggestion scan and Done verification (1, 2, 7, 8).
func BuildPreparePushPrompt(skipMechanical bool) string {
	if !skipMechanical {
		return "/prepare-for-push"
	}
	return "/prepare-for-push Note: lint, typecheck, unit tests, and e2e (steps 3-6) " +
		"already ran and passed via the full mechanical check suite immediately before " +
		"this invocation. Skip those steps and go straight to steps 1, 2, 7, and 8."
}

func agentLabel(agent string) string {
	if agent == "a" {
		return "Google Antigravity"
	}
	return "Claude Code"
}

func (a App) isDirty() bool {
	git := a.GitOut
	if git == nil {
		git = func(dir string, args ...string) (string, error) {
			out, err := worktreeGit(dir, args...)
			return out, err
		}
	}
	out, err := git(a.Root, "status", "--porcelain")
	return err == nil && strings.TrimSpace(out) != ""
}

// commitIfDirty stages and commits whatever an agent left behind, but only if it left something:
// an invocation that made no changes must not produce an empty commit.
func (a App) commitIfDirty(message string) bool {
	if !a.isDirty() {
		return false
	}
	a.git("add", "-A")
	a.git("commit", "-m", message)
	return true
}

// pushWorkflow runs the mechanical gate first (the same one the pre-push hook enforces), and only
// then hands off to the agent for the checks only it can do (doc-sync judgment, suggestion scan,
// Done verification), followed by a human re-run/continue/cancel decision and the actual push. A
// mechanical failure sends the agent in to fix it interactively, never straight to a push. Shared
// by the launcher menu and the standalone prepare_for_push tool.
func (a App) pushWorkflow(ctx context.Context, agent string) (newAgent string, pushed bool) {
	if a.RunTests == nil {
		a.log("\n[ERROR] No automated test runner is configured; refusing to push without running the checks.")
		return agent, false
	}
	failures := a.RunTests("ultra_heavy")
	for len(failures) > 0 {
		if !a.chooseAgent(&agent) {
			return agent, false
		}
		a.log("\nMechanical checks failed:")
		for _, f := range failures {
			a.logf("   - %s", f)
		}
		a.logf("\nLaunching %s interactively to fix failing checks...", agentLabel(agent))
		prompt := fmt.Sprintf("The following automated check(s) failed during prepare-for-push: %s. Please fix the failing checks so that all automated tests pass.", strings.Join(failures, ", "))
		cmd := agentcmd.BuildCommand(agent, agentcmd.CommandOptions{RepoRoot: a.Root, Prompt: prompt, Interactive: true}, a.settings())
		a.runAgent()(ctx, cmd, a.Root)
		if a.commitIfDirty("Fix failing checks before push: " + strings.Join(failures, ", ")) {
			a.log("Committed the agent's fixes.")
		}
		a.log("\nRe-running automated checks...")
		failures = a.RunTests("ultra_heavy")
		if len(failures) > 0 {
			a.log("\nOptions after failing checks:")
			a.log("  (y) Re-run agent to fix failing checks")
			a.log("  (c) Cancel push\n")
			if !a.confirm("Choose option (y/c): ") {
				a.log("\nPush cancelled.")
				return agent, false
			}
		}
	}

	if !a.chooseAgent(&agent) {
		return agent, false
	}
	for again := true; again; {
		a.logf("\nMechanical checks passed. Running prepare-for-push via %s non-interactively...", agentLabel(agent))
		cmd := agentcmd.BuildCommand(agent, agentcmd.CommandOptions{RepoRoot: a.Root, Prompt: BuildPreparePushPrompt(true)}, a.settings())
		a.runAgent()(ctx, cmd, a.Root)
		if a.commitIfDirty("prepare-for-push: apply automated fixes") {
			a.log("Committed the agent's prepare-for-push changes.")
		}
		a.log("\nOptions after prepare command:")
		a.log("  (y) Re-run prepare command")
		a.log("  (n) Continue to git push")
		a.log("  (c) Cancel push\n")
		choice, _ := a.ask("Choose option (y/n/c): ")
		switch strings.ToLower(choice) {
		case "y", "yes":
		case "c", "cancel", "q", "quit":
			a.log("\nPush cancelled.")
			return agent, false
		default:
			again = false
		}
	}
	a.log("\nRunning git push...")
	a.git("push")
	return agent, true
}

// PrepareForPush is the standalone tool: the whole workflow with no menu in front of it. It exits
// 0 only when it pushed. Human-invoked only (the safety hook stops an agent from running it).
func PrepareForPush(root, projectName string, in io.Reader, out io.Writer, runTests func(ctx context.Context, cwd string, layers ...string) []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	app := New(root, projectName, in, out)
	if runTests != nil {
		app.RunTests = func(layers ...string) []string { return runTests(ctx, root, layers...) }
	}
	if _, pushed := app.pushWorkflow(ctx, ""); pushed {
		return 0
	}
	return 1
}

func worktreeGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
