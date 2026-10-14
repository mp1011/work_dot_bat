package planstatus

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// CLI wires the plan-status command to a repo root and output streams. Collect is optional
// (nil = no worktree overlay); the worktree subtask supplies the git-backed implementation.
type CLI struct {
	Root    string
	Stdout  io.Writer
	Stderr  io.Writer
	Collect CollectWorktreeStatuses
	Now     time.Time
}

const usage = "Usage: plan-status.sh <add|add-subtask|set|set-verification|set-plan-order|list|list-tasks|list-changed-done|check|backfill-subtasks|ensure-frontmatter|regenerate> ..."

// Run executes one plan-status command and returns the process exit code.
func (c CLI) Run(argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintf(c.Stderr, "Unknown command: \"undefined\"\n%s\n", usage)
		return 1
	}
	cmd, rest := argv[0], argv[1:]
	code, err := c.dispatch(cmd, rest)
	if err != nil {
		fmt.Fprintln(c.Stderr, err.Error())
		return 1
	}
	return code
}

func (c CLI) printf(format string, a ...any) { fmt.Fprintf(c.Stdout, format, a...) }

func (c CLI) overlaid() []Entry {
	entries := ReadStatus(c.Root)
	if c.Collect == nil {
		return entries
	}
	byPath, err := c.Collect(c.Root)
	if err != nil || byPath == nil {
		return entries
	}
	return ApplyWorktreeStatuses(entries, byPath)
}

func (c CLI) dispatch(cmd string, rest []string) (int, error) {
	switch cmd {
	case "add":
		if len(rest) == 0 {
			return 0, errf("Usage: plan-status.sh add <path> [status]")
		}
		status := "Not Started"
		if len(rest) > 1 {
			status = rest[1]
		}
		if err := Add(c.Root, rest[0], status); err != nil {
			return 0, err
		}
		c.printf("Added %s (%s)\n", rest[0], DeriveEffectiveStatus(NormalizePlanPath(rest[0]), status, false))
	case "add-subtask":
		args := append([]string{}, rest...)
		parent := ""
		for i, a := range args {
			if (a == "-parent" || a == "--parent") && i+1 < len(args) {
				parent = args[i+1]
				args = append(args[:i], args[i+2:]...)
				break
			}
		}
		status := "Not Started"
		if len(args) > 1 {
			status = args[1]
		}
		if len(args) == 0 || parent == "" {
			return 0, errf("Usage: plan-status.sh add-subtask <path> -parent <path to parent> [status]")
		}
		entry, norm := NormalizePlanPath(args[0]), NormalizePlanPath(parent)
		if err := AddSubtask(c.Root, entry, norm, status); err != nil {
			return 0, err
		}
		c.printf("Added subtask %s (subtask of %s) (%s)\n", entry, norm, status)
	case "set":
		human := false
		var pos []string
		for _, a := range rest {
			if a == "--human" {
				human = true
			} else {
				pos = append(pos, a)
			}
		}
		if len(pos) < 2 {
			return 0, errf("Usage: plan-status.sh set <path> <status> [--human]")
		}
		if err := Set(c.Root, pos[0], pos[1], human); err != nil {
			return 0, err
		}
		c.printf("Set %s -> %s\n", pos[0], pos[1])
	case "set-verification":
		if len(rest) < 2 {
			return 0, errf("Usage: plan-status.sh set-verification <path> <manual|automated>")
		}
		if err := SetVerification(c.Root, rest[0], rest[1]); err != nil {
			return 0, err
		}
		c.printf("Set %s verification -> %s\n", rest[0], rest[1])
	case "set-plan-order":
		if len(rest) == 0 {
			return 0, errf("Usage: plan-status.sh set-plan-order <path> [subtask1 subtask2 ...]")
		}
		path := rest[0]
		order := rest[1:]
		if err := SetPlanOrder(c.Root, path, order); err != nil {
			return 0, err
		}
		c.printf("Set plan_order for %s (%d task(s))\n", path, len(order))
	case "regenerate":
		if WriteShim(c.Root) {
			c.printf("Regenerated agent_docs/plan_status.js (%d entries).\n", len(ReadStatus(c.Root)))
		} else {
			c.printf("No agent_docs/plan_index.html in this repo; nothing to regenerate.\n")
		}
	case "list":
		filter := ""
		if len(rest) > 0 {
			filter = rest[0]
			if err := assertFilterableStatus(filter); err != nil {
				return 0, err
			}
		}
		for _, e := range c.overlaid() {
			if filter == "" || e.Status == filter {
				c.printf("%s\t%s\n", e.Status, e.Path)
			}
		}
	case "check":
		return c.check(rest)
	case "backfill-subtasks":
		updated := BackfillSubtasks(c.Root)
		c.printf("Backfilled subtask_of for %d plan file(s).\n", len(updated))
		for _, p := range updated {
			c.printf("  + %s\n", p)
		}
	case "ensure-frontmatter":
		added := EnsureFrontmatter(c.Root)
		c.printf("Added frontmatter to %d plan file(s).\n", len(added))
		for _, p := range added {
			c.printf("  + %s\n", p)
		}
	case "list-tasks":
		c.listTasks(rest)
	case "list-changed-done":
		c.listChangedDone(rest)
	default:
		fmt.Fprintf(c.Stderr, "Unknown command: \"%s\"\n%s\n", cmd, usage)
		return 1, nil
	}
	return 0, nil
}

func (c CLI) check(rest []string) (int, error) {
	opts := CheckOptions{CollectWorktreeStatuses: c.Collect, Now: c.Now}
	staleDays := 0
	if n, err := strconv.Atoi(os.Getenv("PLAN_STATUS_STALE_BLOCKED_DAYS")); err == nil && n > 0 {
		staleDays = n
	}
	for i, a := range rest {
		if a == "--stale-days" && i+1 < len(rest) {
			if n, err := strconv.ParseFloat(rest[i+1], 64); err == nil {
				staleDays = int(n)
			}
		}
	}
	opts.StaleDays = staleDays
	r, err := Check(c.Root, opts)
	if err != nil {
		return 0, err
	}
	if len(r.MissingFrontmatter) > 0 {
		c.printf("Plan files with no status frontmatter (%d):\n", len(r.MissingFrontmatter))
		for _, f := range r.MissingFrontmatter {
			c.printf("  + %s\n", f)
		}
	}
	if len(r.InvalidStatus) > 0 {
		c.printf("Plan files with an invalid status (%d):\n", len(r.InvalidStatus))
		for _, f := range r.InvalidStatus {
			c.printf("  ! %s (%s)\n", f.Path, f.Status)
		}
	}
	if len(r.BlockedByErrors) > 0 {
		c.printf("Plan files with invalid blocked_by dependencies (%d):\n", len(r.BlockedByErrors))
		for _, f := range r.BlockedByErrors {
			c.printf("  ! %s %s\n", f.Path, f.Error)
		}
	}
	if len(r.SubtaskErrors) > 0 {
		c.printf("Plan files with missing/incorrect subtask_of in plan.md folders (%d):\n", len(r.SubtaskErrors))
		for _, f := range r.SubtaskErrors {
			c.printf("  ! %s %s\n", f.Path, f.Error)
		}
	}
	if len(r.WorktreeDivergence) > 0 {
		c.printf("Plans whose worktree branch disagrees with main (%d):\n", len(r.WorktreeDivergence))
		for _, d := range r.WorktreeDivergence {
			behind := ""
			if d.CommitsBehind > 0 {
				behind = fmt.Sprintf(" (%d behind)", d.CommitsBehind)
			}
			c.printf("  ! %s — main: %s, %s%s: %s\n", d.Path, d.MainStatus, d.Branch, behind, d.BranchStatus)
		}
	}
	if len(r.OpenSubtasksForDoneParents) > 0 {
		c.printf("Parent plans marked \"Done\" with open subtasks (%d):\n", len(r.OpenSubtasksForDoneParents))
		for _, d := range r.OpenSubtasksForDoneParents {
			c.printf("  ! %s is \"Done\" but subtask %s is \"%s\"\n", d.Parent, d.Subtask, d.SubtaskStatus)
		}
	}
	if r.OK {
		c.printf("All tracked plan files have valid status frontmatter.\n")
	}
	if len(r.StaleBlockedPlans) > 0 {
		days := staleDays
		if days == 0 {
			days = DefaultStaleBlockedDays
		}
		c.printf("Warning: plan files in \"Blocked - User Input Required\" for more than %d days (%d):\n", days, len(r.StaleBlockedPlans))
		for _, s := range r.StaleBlockedPlans {
			c.printf("  ! %s (blocked for %dd without human action)\n", s.Path, s.Days)
		}
	}
	if !HasOpenBacklog(ReadStatus(c.Root)) {
		c.printf("Warning: no entries are \"Not Started\" or \"In Progress\" — the backlog is empty. " +
			"If a health assessment ran recently, confirm its recommendations were registered.\n")
	}
	if r.OK {
		return 0, nil
	}
	return 1, nil
}
