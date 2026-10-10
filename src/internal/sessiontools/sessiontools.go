// Package sessiontools is the `analyze_session` and `reclassify_sessions` tools: read the session
// ledger and agent logs/transcripts, print what happened, and (for reclassify) relabel ledger rows.
package sessiontools

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"work_dot_bat/internal/agentsession"
	"work_dot_bat/internal/planclaims"
	"work_dot_bat/internal/repo"
)

// Config is the process-level context for both tools.
type Config struct {
	Root     string // the selected project's root
	Out, Err io.Writer
	Now      func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

const ledgerName = repo.StateDirName + "/session_ledger.json"

const analyzeUsage = `
Usage:
  work_dot_bat analyze_session project=<name> [--latest]
  work_dot_bat analyze_session project=<name> <plan-path>
  work_dot_bat analyze_session project=<name> <session-name>

Options:
  --latest, -l   Analyze the most recent session from the ledger (default if no args)
  --help, -h     Show this help message
`

func has(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func (c Config) analyzeOne(s agentsession.Entry) {
	var content string
	if logFile, _ := s["logFile"].(string); logFile != "" {
		resolved := logFile
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(c.Root, resolved)
		}
		if b, err := os.ReadFile(resolved); err == nil {
			content = string(b)
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(c.Err, "[WARN] Could not read log file at %s: %v\n", resolved, err)
		}
	}
	fmt.Fprintln(c.Out, agentsession.FormatSessionReport(s, agentsession.ParseLogContent(content)))
	fmt.Fprintln(c.Out)
}

// Analyze prints the report for the latest session, a named session, or every session of a plan.
func Analyze(c Config, args []string) int {
	ledgerFile := filepath.Join(c.Root, filepath.FromSlash(ledgerName))
	ledger := agentsession.ReadLedger(ledgerFile)
	if has(args, "--help", "-h") {
		fmt.Fprint(c.Out, analyzeUsage+"\n")
		return 0
	}
	if len(ledger) == 0 {
		fmt.Fprintf(c.Out, "No sessions recorded yet in %s.\n", ledgerName)
		return 0
	}
	target := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			target = a
			break
		}
	}
	if target == "" || has(args, "--latest", "-l") {
		latest := ledger[len(ledger)-1]
		fmt.Fprintf(c.Out, "Analyzing latest session (%v):\n\n", latest["sessionName"])
		c.analyzeOne(latest)
		return 0
	}
	for _, s := range ledger {
		if s["sessionName"] == target {
			fmt.Fprintf(c.Out, "Found session matching name \"%s\":\n\n", target)
			c.analyzeOne(s)
			return 0
		}
	}
	if matches := agentsession.FindSessionsForPlan(ledgerFile, target); len(matches) > 0 {
		fmt.Fprintf(c.Out, "Found %d session(s) for plan \"%s\":\n\n", len(matches), target)
		for _, s := range matches {
			c.analyzeOne(s)
		}
		return 0
	}
	fmt.Fprintf(c.Out, "No sessions found matching \"%s\".\n", target)
	return 0
}

const reclassifyUsage = `Usage: work_dot_bat reclassify_sessions project=<name> [options]

Options:
  --write                Rewrite matching sessions in the ledger file
  --orphans              Look for orphaned in-progress sessions (>24h) instead of rate-limits
  --ledger <file>        Explicit path to session ledger JSON file
  --projects-dir <dir>   Path to projects directory containing agent transcripts
  --help, -h             Show this help message
`

func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// resolveLedger: an explicit path, else this checkout's ledger, else the main checkout's (a
// worktree's own state dir is usually empty).
func (c Config) resolveLedger(args []string) string {
	if explicit := argValue(args, "--ledger"); explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return explicit
		}
		return abs
	}
	local := filepath.Join(c.Root, filepath.FromSlash(ledgerName))
	if _, err := os.Stat(local); err == nil {
		return local
	}
	if main := repo.FindMainRepoRoot(c.Root); main != "" && main != c.Root {
		if p := filepath.Join(main, filepath.FromSlash(ledgerName)); fileExists(p) {
			return p
		}
	}
	return local
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// liveClaimChecker says whether a row's plan has a held claim in this or the main checkout.
func (c Config) liveClaimChecker() func(*agentsession.Row) bool {
	roots := []string{c.Root}
	if main := repo.FindMainRepoRoot(c.Root); main != "" && main != c.Root {
		roots = append(roots, main)
	}
	env := planclaims.DefaultEnv()
	var held []string
	for _, r := range roots {
		for _, cl := range env.ListClaims(r) {
			if cl.Decision.State == planclaims.Held {
				held = append(held, agentsession.CleanPlanSlug(cl.ID))
			}
		}
	}
	return func(r *agentsession.Row) bool {
		plan := r.Str("plan")
		if plan == "" {
			return false
		}
		slug := agentsession.CleanPlanSlug(plan)
		for _, h := range held {
			if h == slug {
				return true
			}
		}
		return false
	}
}

// Reclassify relabels ledger rows: completed-but-really-rate-limited ones, or (--orphans) stale
// in-progress ones. Dry run unless --write.
func Reclassify(c Config, args []string) int {
	if has(args, "--help", "-h") {
		fmt.Fprint(c.Out, reclassifyUsage)
		return 0
	}
	ledgerFile := c.resolveLedger(args)
	write, orphans := has(args, "--write"), has(args, "--orphans")
	if !fileExists(ledgerFile) {
		fmt.Fprintf(c.Err, "No session ledger at %s\n", ledgerFile)
		return 1
	}
	rows := agentsession.ReadRows(ledgerFile)
	total := len(rows)

	var changed []string
	var what, relabel string
	if orphans {
		changed = agentsession.ReclassifyOrphans(rows, c.liveClaimChecker(), c.now(), 24*time.Hour)
		what, relabel = "orphaned in-progress session(s)", "abandoned"
	} else {
		dirs := agentsession.Dirs{Projects: argValue(args, "--projects-dir")}
		changed = agentsession.ReclassifyLedger(rows, func(r *agentsession.Row) agentsession.Verdict {
			// A reset time like "1:30pm" is only meaningful relative to when the session ended,
			// not to now.
			at := c.now()
			for _, k := range []string{"endedAt", "startedAt"} {
				if t, err := time.Parse(time.RFC3339Nano, r.Str(k)); err == nil {
					at = t
					break
				}
			}
			exit := 0
			if v, ok := r.Get("exitCode"); ok {
				if f, ok := v.(interface{ Int64() (int64, error) }); ok {
					n, _ := f.Int64()
					exit = int(n)
				}
			}
			return agentsession.ClassifySession(r.Str("agent"), r.Str("sessionId"), "", exit, dirs, at)
		})
		what, relabel = "bounced off a usage limit", "rate-limited"
	}
	fmt.Fprintf(c.Out, "%d session(s) in %s; %d %s:\n", total, ledgerFile, len(changed), what)
	for _, name := range changed {
		fmt.Fprintf(c.Out, "  %s\n", name)
	}
	if len(changed) == 0 {
		return 0
	}
	if !write {
		fmt.Fprintf(c.Out, "\nDry run — re-run with --write to relabel them `%s`.\n", relabel)
		return 0
	}
	if err := agentsession.WriteRows(ledgerFile, rows); err != nil {
		fmt.Fprintf(c.Err, "Could not write %s: %v\n", ledgerFile, err)
		return 1
	}
	fmt.Fprintf(c.Out, "\nRewrote %s.\n", ledgerFile)
	return 0
}
