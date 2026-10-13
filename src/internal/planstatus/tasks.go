package planstatus

import (
	"fmt"
	"os"
	"strings"
)

type Task struct {
	Entry
}

// OverlaidEntries is every tracked plan overlaid with worktree status.
func (c CLI) OverlaidEntries() []Entry { return c.overlaid() }

// ListOpenTasks is the open backlog overlaid with worktree status (so a plan finished on a
// branch reads as its effective status). Filtering open/closed happens after the overlay.
func (c CLI) ListOpenTasks() []Task {
	var tasks []Task
	for _, e := range c.OverlaidEntries() {
		if e.Status == "Done" || e.Status == "Cancelled" || e.Status == "Suggestion" {
			continue
		}
		tasks = append(tasks, Task{Entry: e})
	}
	return tasks
}

// FormatTaskLine renders "[status @ branch] path" .
func FormatTaskLine(t Task) string {
	branch := ""
	if t.WorktreeBranch != "" {
		branch = " @ " + t.WorktreeBranch
	}
	return fmt.Sprintf("[%s%s] %s", t.Status, branch, t.Path)
}

// RenderTaskList is the numbered backlog plus an unnumbered footer when suggestions are parked.
func RenderTaskList(tasks []Task, parked int) string {
	var b strings.Builder
	if len(tasks) == 0 {
		b.WriteString("No open tasks found.\n")
	}
	for i, t := range tasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, FormatTaskLine(t))
	}
	if parked > 0 {
		noun := "suggestions"
		if parked == 1 {
			noun = "suggestion"
		}
		fmt.Fprintf(&b, "%d parked %s — scripts/work_dot_bat/plan-status.sh list Suggestion\n", parked, noun)
	}
	return b.String()
}

func (c CLI) listTasks(rest []string) {
	tasks := c.ListOpenTasks()
	for _, a := range rest {
		if a == "--free" {
			var free []Task
			for _, t := range tasks {
				if t.Status == "Not Started" {
					free = append(free, t)
				}
			}
			tasks = free
			break
		}
	}
	parked := 0
	for _, e := range c.OverlaidEntries() {
		if e.Status == "Suggestion" {
			parked++
		}
	}
	fmt.Fprint(c.Stdout, RenderTaskList(tasks, parked))
}

type ChangedPlan struct{ Path, Status string }

// ListChangedDone lists plan files changed against baseRef that are Done or Ready for Review.
// If the diff fails (unknown ref) it falls back to the working-tree status.
func ListChangedDone(root, baseRef string, git GitRunner) []ChangedPlan {
	out, err := git(root, "diff", "--name-only", baseRef)
	if err != nil {
		out = ""
		if st, err := git(root, "status", "--porcelain"); err == nil {
			var names []string
			for _, line := range strings.Split(st, "\n") {
				if len(line) >= 3 {
					names = append(names, line[3:])
				} else {
					names = append(names, "")
				}
			}
			out = strings.Join(names, "\n")
		}
	}
	var results []ChangedPlan
	for _, rel := range PlanPathsFromGitOutput(out) {
		content, err := os.ReadFile(docsPath(root, rel))
		if err != nil {
			continue
		}
		if fm := ParseFrontmatter(string(content)); fm.Status != nil && (*fm.Status == "Done" || *fm.Status == "Ready for Review") {
			results = append(results, ChangedPlan{rel, *fm.Status})
		}
	}
	return results
}

func (c CLI) listChangedDone(rest []string) {
	base := "origin/main"
	if len(rest) > 0 {
		base = rest[0]
	}
	plans := ListChangedDone(c.Root, base, defaultGit)
	if len(plans) == 0 {
		fmt.Fprintf(c.Stdout, "No changed \"Done\" or \"Ready for Review\" plans found (against %s).\n", base)
		return
	}
	for _, p := range plans {
		fmt.Fprintf(c.Stdout, "%s\t%s\n", p.Status, p.Path)
	}
}
