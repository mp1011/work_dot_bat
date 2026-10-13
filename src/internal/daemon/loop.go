package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"work_dot_bat/src/internal/planstatus"
)

const PollInterval = time.Minute

// Loop is the daemon's unattended main loop: read the plans, work the first eligible one, repeat
// until cancelled.
type Loop struct {
	Runner Runner
	Out    io.Writer
	// PollInterval is how long to wait when idle or blocked; 0 = PollInterval.
	PollInterval time.Duration
	// MaxIterations stops the loop (tests); 0 = forever.
	MaxIterations int

	// OpenBlocked opens an editor on a blocked plan's worktree, once per plan; nil = never.
	OpenBlocked func(worktreePath string)

	// Seams: defaults read real state.
	ReadPlans func() []Plan
	Committed func(plans []Plan) func(Plan) bool
}

func (l Loop) stamp(msg string) {
	fmt.Fprintf(l.Out, "[%s] %s\n", l.Runner.now().Format("15:04:05"), msg)
}

func (l Loop) plans() []Plan {
	if l.ReadPlans != nil {
		return l.ReadPlans()
	}
	wts, _ := existingWorktrees(l.Runner.Root)
	return ReadPlanStates(l.Runner.Root, wts, l.Runner.ReadPlans)
}

func (l Loop) committed(plans []Plan) func(Plan) bool {
	if l.Committed != nil {
		return l.Committed(plans)
	}
	set := CommittedPlanPaths(l.Runner.Root, plans, l.Runner.GitRead)
	return func(p Plan) bool { return set[p.Path] }
}

// Run loops until ctx is cancelled (exit 0) or an agent gets stuck past its timeout (exit 1: the
// agent CLI itself is wedged, so carrying on would only wedge the next plan).
func (l Loop) Run(ctx context.Context) int {
	poll := l.PollInterval
	if poll == 0 {
		poll = PollInterval
	}
	opened := map[string]bool{}
	for i := 0; ctx.Err() == nil && (l.MaxIterations == 0 || i < l.MaxIterations); i++ {
		plans := l.plans()
		res := Eligibility(plans, Inputs{Committed: l.committed(plans)})

		if len(res.Blocked) > 0 {
			l.stamp("")
			fmt.Fprintln(l.Out, "===================================================")
			fmt.Fprintf(l.Out, "BLOCKED — daemon will not proceed (%d):\n", len(res.Blocked))
			for _, b := range res.Blocked {
				branch := DaemonBranchFor(b.Path, plans)
				where := "(branch: " + branch + ")"
				if b.Source != "" && b.Source != "main" {
					where = b.Source
				}
				reason := b.BlockedReason
				if reason == "" {
					reason = "Blocked - User Input Required"
				}
				fmt.Fprintf(l.Out, "  Plan:     %s\n  Worktree: %s\n  Reason:   %s\n", b.Path, where, reason)
				fmt.Fprintln(l.Out, "---------------------------------------------------")
				if l.OpenBlocked != nil && b.Source != "" && b.Source != "main" && !opened[b.Path] {
					opened[b.Path] = true
					l.stamp("Opening VS Code for worktree " + where + "...")
					l.OpenBlocked(b.Source)
				}
			}
			fmt.Fprintln(l.Out, "Resolve the blockers in the worktree, then change status to resume.")
			fmt.Fprintln(l.Out, "===================================================")
			fmt.Fprintln(l.Out)
			if l.Runner.sleep(ctx, poll) != nil {
				break
			}
			continue
		}
		if len(res.Eligible) == 0 {
			l.stamp("Idle — no eligible plans. Waiting...")
			if l.Runner.sleep(ctx, poll) != nil {
				break
			}
			continue
		}
		next := res.Eligible[0]
		l.stamp(fmt.Sprintf("Next eligible plan: %s (%s)", next.Path, next.Status))
		r := l.Runner
		r.Log = l.stamp
		out := r.RunPlan(ctx, next, plans)
		if out.Status == "timed-out" {
			return 1
		}
	}
	return 0
}

// FormatReason is a one-line summary of a plan result for logs.
func FormatReason(r PlanResult) string {
	parts := []string{r.Status}
	if r.Reason != "" {
		parts = append(parts, r.Reason)
	}
	return strings.Join(parts, ": ")
}

var existingWorktrees = func(root string) ([]planstatus.Worktree, error) { return worktreeList(root) }

var timeNow = time.Now
