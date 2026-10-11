package planclaims

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"work_dot_bat/src/internal/planstatus"
)

// CLI is the agent-facing plan-claim command: argv parsing and output formatting only.
type CLI struct {
	Root    string
	Stdout  io.Writer
	Stderr  io.Writer
	Env     Env
	Collect planstatus.CollectWorktreeStatuses // nil = no worktree overlay
}

func (c CLI) printf(format string, a ...any) { fmt.Fprintf(c.Stdout, format, a...) }

func (c CLI) resolve(rawPlan string) (planID, branch string, err error) {
	planID = planstatus.NormalizePlanPath(rawPlan)
	if planID == "" {
		return "", "", fmt.Errorf("A plan path is required")
	}
	return planID, planstatus.BranchNameForPlan(planID, planstatus.ReadStatus(c.Root)), nil
}

func scopesFor(planID, branch, worktree string) []Scope {
	return []Scope{
		{Scope: "plan", ID: planID, Branch: branch, Worktree: worktree},
		{Scope: "branch", ID: branch, Branch: branch, Worktree: worktree},
	}
}

func freshnessText(d Decision) string {
	if d.State == Held {
		return "fresh"
	}
	return d.Reason
}

func (c CLI) age(r Claim) time.Duration {
	if t, ok := parseTime(r.Since()); ok {
		return c.Env.Now().Sub(t)
	}
	return time.Duration(1<<63 - 1)
}

// detailBlock is the HELD / OVERRIDING block shown for a claim.
func (c CLI) detailBlock(label, planID string, r Claim, d Decision) string {
	wt := "(none)"
	if r.Worktree != nil {
		wt = *r.Worktree
	}
	return strings.Join([]string{
		fmt.Sprintf("%s  %s", label, planID),
		fmt.Sprintf("      owner     %s", DescribeOwner(r.Owner)),
		fmt.Sprintf("      branch    %s", r.Branch),
		fmt.Sprintf("      worktree  %s", wt),
		fmt.Sprintf("      age       %s (ttl %s — %s)", FormatAge(c.age(r)), FormatAge(r.ttl()), freshnessText(d)),
	}, "\n")
}

// dedupeByClaimEvent collapses scopes acquired together (same owner + acquiredAt).
func dedupeByClaimEvent(found []*Found) []*Found {
	seen := map[string]bool{}
	var out []*Found
	for _, f := range found {
		o := f.Record.Owner
		key := fmt.Sprintf("%s|%d|%s|%s", o.Kind, o.PID, o.Host, f.Record.AcquiredAt)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

func (c CLI) blocking(scopes []Scope, onlyBlocking bool) []*Found {
	var out []*Found
	for _, s := range scopes {
		f := c.Env.ReadClaimWithDecision(c.Root, s.Scope, s.ID)
		if f == nil {
			continue
		}
		if onlyBlocking && f.Decision.State != Held && f.Decision.State != StaleReport {
			continue
		}
		out = append(out, f)
	}
	return out
}

type flags struct {
	owner, worktree, kind string
	pid                   *string
	steal                 bool
}

func parseFlags(args []string) (positional []string, f flags) {
	f.kind = "agent"
	next := func(i *int) string {
		*i++
		if *i < len(args) {
			return args[*i]
		}
		return ""
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--owner":
			f.owner = next(&i)
		case "--worktree":
			f.worktree = next(&i)
		case "--steal":
			f.steal = true
		case "--kind":
			f.kind = next(&i)
		case "--pid":
			v := next(&i)
			f.pid = &v
		default:
			positional = append(positional, args[i])
		}
	}
	return
}

// Run executes one plan-claim command and returns the process exit code.
func (c CLI) Run(argv []string) int {
	var cmd string
	var rest []string
	if len(argv) > 0 {
		cmd, rest = argv[0], argv[1:]
	}
	code, err := c.dispatch(cmd, rest, len(argv) > 0)
	if err != nil {
		fmt.Fprintln(c.Stderr, err.Error())
		return 1
	}
	return code
}

func (c CLI) dispatch(cmd string, rest []string, hasCmd bool) (int, error) {
	switch cmd {
	case "check":
		return c.check(rest)
	case "acquire":
		return c.acquire(rest)
	case "renew":
		return c.renew(rest)
	case "release":
		return c.release(rest)
	case "list":
		return c.list(), nil
	case "prune":
		return c.prune(), nil
	}
	name := "undefined"
	if hasCmd {
		name = cmd
	}
	fmt.Fprintf(c.Stderr, "Unknown command: \"%s\"\nUsage: plan-claim.sh <check|acquire|renew|release|list|prune> ...\n", name)
	return 1, nil
}

func (c CLI) check(rest []string) (int, error) {
	if len(rest) == 0 {
		return 0, fmt.Errorf("Usage: plan-claim.sh check <plan>")
	}
	planID, branch, err := c.resolve(rest[0])
	if err != nil {
		return 0, err
	}
	blocking := c.blocking(scopesFor(planID, branch, ""), true)
	if len(blocking) == 0 {
		c.printf("FREE  %s\n", planID)
		return 0, nil
	}
	var blocks []string
	for _, b := range dedupeByClaimEvent(blocking) {
		blocks = append(blocks, c.detailBlock("HELD", planID, b.Record, b.Decision))
	}
	c.printf("%s\n", strings.Join(blocks, "\n\n"))
	return 1, nil
}


var digitsRE = regexp.MustCompile(`^\d+$`)

func (c CLI) acquire(rest []string) (int, error) {
	positional, f := parseFlags(rest)
	if len(positional) == 0 || f.owner == "" {
		return 0, fmt.Errorf("Usage: plan-claim.sh acquire <plan> --owner <label> [--worktree <path>] [--steal] [--kind daemon|agent|launcher] [--pid <n>]")
	}
	if f.kind != "daemon" && f.kind != "agent" && f.kind != "launcher" {
		return 0, fmt.Errorf("Invalid --kind \"%s\". Allowed: daemon, agent, launcher", f.kind)
	}
	if f.pid != nil && !digitsRE.MatchString(*f.pid) {
		return 0, fmt.Errorf("Invalid --pid \"%s\". Expected a positive integer.", *f.pid)
	}
	planID, branch, err := c.resolve(positional[0])
	if err != nil {
		return 0, err
	}
	scopes := scopesFor(planID, branch, f.worktree)
	// --pid lets a long-lived launcher record its own pid instead of this ephemeral process's,
	// so a launcher claim left behind by a crash is pid-reclaimable.
	pid := c.Env.PID
	if f.pid != nil {
		pid, _ = strconv.Atoi(*f.pid)
	}
	owner := Owner{Kind: f.kind, Label: f.owner, PID: pid, Host: c.Env.Hostname}
	ttl := AgentTTL
	if f.kind == "daemon" {
		ttl = DaemonTTL
	}

	if f.steal {
		// The only destructive path: show whatever is held before overriding it.
		for _, h := range dedupeByClaimEvent(c.blocking(scopes, false)) {
			c.printf("%s\n\n", c.detailBlock("OVERRIDING", planID, h.Record, h.Decision))
		}
	}
	res, err := c.Env.Acquire(c.Root, scopes, owner, ttl, f.steal)
	if err != nil {
		return 0, err
	}
	if !res.OK {
		// Re-read the blocking record purely for display.
		if found := c.Env.ReadClaimWithDecision(c.Root, res.BlockedScope, res.BlockedID); found != nil {
			c.printf("%s\n", c.detailBlock("HELD", planID, found.Record, res.Decision))
		} else {
			c.printf("HELD  %s\n", planID)
		}
		return 1, nil
	}
	c.printf("ACQUIRED  %s  (branch %s, ttl %s)\n", planID, branch, FormatAge(ttl))
	return 0, nil
}

func (c CLI) renew(rest []string) (int, error) {
	if len(rest) == 0 {
		return 0, fmt.Errorf("Usage: plan-claim.sh renew <plan>")
	}
	planID, branch, err := c.resolve(rest[0])
	if err != nil {
		return 0, err
	}
	for _, s := range []Scope{{Scope: "plan", ID: planID}, {Scope: "branch", ID: branch}} {
		if _, err := c.Env.Renew(c.Root, s.Scope, s.ID); err != nil {
			fmt.Fprintf(c.Stderr, "Cannot renew %s claim for \"%s\": %s\n", s.Scope, s.ID, err.Error())
			return 1, nil
		}
	}
	c.printf("RENEWED  %s\n", planID)
	return 0, nil
}

func (c CLI) release(rest []string) (int, error) {
	if len(rest) == 0 {
		return 0, fmt.Errorf("Usage: plan-claim.sh release <plan>")
	}
	planID, branch, err := c.resolve(rest[0])
	if err != nil {
		return 0, err
	}
	if err := Release(c.Root, "plan", planID); err != nil {
		return 0, err
	}
	if err := Release(c.Root, "branch", branch); err != nil {
		return 0, err
	}
	c.printf("RELEASED  %s\n", planID)
	return 0, nil
}

func (c CLI) list() int {
	claims := c.Env.ListClaims(c.Root)
	if len(claims) == 0 {
		c.printf("No live claims.\n")
		return 0
	}
	var sw, iw, kw int
	for _, cl := range claims {
		sw, iw, kw = max(sw, len(cl.Scope)), max(iw, len(cl.ID)), max(kw, len(cl.Owner.Kind))
	}
	for _, cl := range claims {
		status := "fresh"
		if cl.Decision.State != Held {
			status = "STALE — " + cl.Decision.Reason
		}
		c.printf("%-*s  %-*s  %-*s  %-4s  %s\n", sw, cl.Scope, iw, cl.ID, kw, cl.Owner.Kind, FormatAge(c.age(cl.Claim)), status)
	}
	return 0
}

// prune removes every claim `list` would show as reclaimable; a stale-report claim is named
// but left alone (`acquire --steal` is the deliberate way to override one).
func (c CLI) prune() int {
	res := c.Env.Prune(c.Root, "")
	for _, r := range res.Removed {
		c.printf("PRUNED  %s %s  — %s: %s\n", r.Scope, r.ID, DescribeOwner(r.Owner), r.Decision.Reason)
	}
	for _, k := range res.Kept {
		if k.Decision.State == StaleReport {
			c.printf("KEPT    %s %s  — %s\n", k.Scope, k.ID, k.Decision.Reason)
		}
	}
	if len(res.Removed) == 0 {
		c.printf("Nothing to prune.\n")
	}
	return 0
}

// ListTaskClaims indexes every plan-scope claim by plan path for `list-tasks`, rendering the
// "CLAIMED by <kind> (pid N) <age> ago" phrase against the env's clock.
func (e Env) ListTaskClaims(root string) map[string]planstatus.TaskClaim {
	out := map[string]planstatus.TaskClaim{}
	for _, c := range e.ListClaims(root) {
		if c.Scope != "plan" {
			continue
		}
		age := time.Duration(1<<63 - 1)
		if t, ok := parseTime(c.Since()); ok {
			age = e.Now().Sub(t)
		}
		out[c.ID] = planstatus.TaskClaim{
			Held: c.Decision.State == Held,
			Text: fmt.Sprintf("CLAIMED by %s (pid %d) %s ago", c.Owner.Kind, c.Owner.PID, FormatAge(age)),
		}
	}
	return out
}
