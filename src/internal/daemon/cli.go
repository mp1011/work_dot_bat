package daemon

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/worktree"
)

// Config is what the process-level entry points need from the rest of the program.
type Config struct {
	Root        string
	ProjectName string
	In          io.Reader
	Out         io.Writer
	// RunTests runs the project's automated checks in cwd, returning the failing check names.
	RunTests func(ctx context.Context, cwd string, layers ...string) []string
	// OpenEditor opens a folder in the user's editor; nil = never.
	OpenEditor func(path string)
}

// Status prints the `auto_status` report: which `auto: true` plans the daemon would pick up and
// why each of the others is held back. Read-only; always exit 0.
func Status(c Config) int {
	wts, _ := worktreeList(c.Root)
	plans := ReadPlanStates(c.Root, wts, nil)
	committed := CommittedPlanPaths(c.Root, plans, nil)
	claims := planclaims.DefaultEnv().ListClaims(c.Root)
	res := Eligibility(plans, Inputs{Committed: func(p Plan) bool { return committed[p.Path] }, Claims: claims})
	fmt.Fprintln(c.Out, FormatAutoStatusReport(res, plans))
	return 0
}

// Main runs the unattended daemon until SIGINT/SIGTERM. It asks exactly one question — which
// agent to use — and then runs on its own.
func Main(c Config) int {
	fmt.Fprintln(c.Out, "Auto Daemon")
	fmt.Fprintln(c.Out, "-----------")
	reader := bufio.NewReader(c.In)
	io := menu.IO{
		Ask: func(prompt string) (string, bool) {
			fmt.Fprint(c.Out, prompt)
			line, err := reader.ReadString('\n')
			if err != nil && line == "" {
				return "", false
			}
			return line, true
		},
		Log: func(s string) { fmt.Fprintln(c.Out, s) },
	}
	agent, quit, ok := agentcmd.ChooseAgent("", io, agentcmd.ProjectSettings(c.Root, c.Root).Defs)
	if quit || !ok {
		return 0
	}

	env := planclaims.DefaultEnv()
	stamp := func(msg string) { fmt.Fprintf(c.Out, "[%s] %s\n", timeNow().Format("15:04:05"), msg) }
	if removed := env.PruneStaleClaims(c.Root, func(m string) { fmt.Fprintln(c.Out, m) }); len(removed) > 0 {
		stamp(fmt.Sprintf("Pruned %d stale claim(s) at startup.", len(removed)))
	}
	label := agentcmd.ProjectSettings(c.Root, c.Root).AgentLabel(agent)
	stamp(fmt.Sprintf("Starting unattended daemon loop with agent: %s...", label))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stamp("Received a stop signal. Releasing claims and shutting down...")
	}()

	runner := Runner{
		Root: c.Root, ProjectName: c.ProjectName, Agent: agent,
		DaemonDoc: projectconfig.DaemonDoc(c.Root, c.Root),
		Log:       stamp, ClaimEnv: env, Ledger: repo.LedgerPath(c.Root),
		Dirs:     agentsession.Dirs{},
		Settings: func() agentcmd.Settings { return agentcmd.ProjectSettings(c.Root, c.Root) },
		RunTests: c.RunTests,
		Bootstrap: func(path string) bool {
			p, err := worktree.LoadProvisioning(path, c.Root)
			if err != nil {
				stamp("[ERROR] " + err.Error())
				return false
			}
			return worktree.Bootstrap(path, p, worktree.RunCommand, stamp)
		},
		StartDevServer: func(path string) {
			if p, err := worktree.LoadProvisioning(path, c.Root); err == nil {
				if _, err := worktree.StartDevServer(path, p, stamp); err != nil {
					stamp("[WARN] dev server did not start: " + err.Error())
				}
			}
		},
	}
	return Loop{Runner: runner, Out: c.Out, OpenBlocked: c.OpenEditor}.Run(ctx)
}
