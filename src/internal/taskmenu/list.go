// Package taskmenu is the interactive "Step 1" task picker: which plans are listed, in what
// order and format, and the prompt loop (write-in, paste a path, parked suggestions) around
// them. Input goes through IO so the menu is testable without a TTY.
package taskmenu

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"work_dot_bat/internal/planstatus"
)

// Item is a plan as the menu lists it; IsSubtask marks a row grouped under its parent.
type Item struct {
	planstatus.Entry
	IsSubtask bool
}

var statusColorCodes = map[string]string{
	"Not Started":      "90",
	"In Progress":      "33",
	"Ready for Review": "36",
	"Done":             "32",
	"Cancelled":        "31",
	"Suggestion":       "35",
}

// ColorizeStatus wraps text in the status's ANSI color, only on a terminal so captured logs
// never contain escape codes.
func ColorizeStatus(text, status string, isTTY bool) string {
	code, ok := statusColorCodes[status]
	if !ok || !isTTY {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func toSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

func IsOldPlansFile(p string) bool {
	if p == "" {
		return false
	}
	n := strings.TrimPrefix(toSlash(p), "agent_docs/")
	return strings.HasPrefix(n, "plans/old_plans/") || strings.HasPrefix(n, "old_plans/") || strings.Contains(n, "/old_plans/")
}

var archiveDirRE = regexp.MustCompile(`^plans/(old_plans|done)/`)

func normalizeForDoneCheck(p string) string {
	if p == "" {
		return ""
	}
	return archiveDirRE.ReplaceAllString(strings.TrimPrefix(toSlash(p), "agent_docs/"), "plans/")
}

func fileIsDone(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fm := planstatus.ParseFrontmatter(string(b))
	return fm.Status != nil && *fm.Status == "Done"
}

// IsPlanMarkedDoneInMain reports whether the plan is Done as seen from the main checkout
// (an entry without a worktree branch, an archived copy, or its own file).
func IsPlanMarkedDoneInMain(task planstatus.Entry, all []planstatus.Entry, root string) bool {
	norm := normalizeForDoneCheck(task.Path)
	for _, t := range all {
		if t.WorktreeBranch == "" && t.Status == "Done" && normalizeForDoneCheck(t.Path) == norm {
			return true
		}
	}
	if root == "" || !strings.HasPrefix(norm, "plans/") {
		return false
	}
	sub := strings.TrimPrefix(norm, "plans/")
	docs := filepath.Join(root, "agent_docs")
	if _, err := os.Stat(filepath.Join(docs, "plans", "done", filepath.FromSlash(sub))); err == nil {
		return true
	}
	return fileIsDone(filepath.Join(docs, "plans", "old_plans", filepath.FromSlash(sub))) ||
		fileIsDone(filepath.Join(docs, filepath.FromSlash(norm)))
}

func isResolved(status string) bool {
	return status == "Ready for Review" || status == "Done" || status == "Cancelled"
}

// RunnableAndGrouped filters the plan set to the runnable backlog and orders it: each parent
// followed by its subtasks (by priority, P1 first, unprioritised last in scan order).
func RunnableAndGrouped(tasks []planstatus.Entry, root string) []Item {
	statusOf := map[string]string{}
	for _, t := range tasks {
		statusOf[t.Path] = t.Status
	}
	var runnable []planstatus.Entry
	for _, t := range tasks {
		if IsOldPlansFile(t.Path) || t.Status == "Cancelled" || t.Status == "Suggestion" {
			continue
		}
		if t.Status == "Done" && (t.WorktreeBranch == "" || IsPlanMarkedDoneInMain(t, tasks, root)) {
			continue
		}
		if t.DependsOn != "" && t.Status != "Done" && !isResolved(statusOf[t.DependsOn]) {
			continue
		}
		runnable = append(runnable, t)
	}
	inRunnable := map[string]bool{}
	for _, t := range runnable {
		inRunnable[t.Path] = true
	}
	subtasks := map[string][]planstatus.Entry{}
	for _, t := range runnable {
		if t.SubtaskOf != "" && inRunnable[t.SubtaskOf] {
			subtasks[t.SubtaskOf] = append(subtasks[t.SubtaskOf], t)
		}
	}
	rank := func(e planstatus.Entry) int {
		if e.Priority == 0 {
			return 1 << 30
		}
		return e.Priority
	}
	var ordered []Item
	for _, t := range runnable {
		if t.SubtaskOf != "" && inRunnable[t.SubtaskOf] {
			continue
		}
		ordered = append(ordered, Item{Entry: t})
		subs := subtasks[t.Path]
		sort.SliceStable(subs, func(i, j int) bool { return rank(subs[i]) < rank(subs[j]) })
		for _, s := range subs {
			ordered = append(ordered, Item{Entry: s, IsSubtask: true})
		}
	}
	return ordered
}

// ParkedSuggestions are the plans under agent_suggestions/, sorted by path.
func ParkedSuggestions(tasks []planstatus.Entry) []planstatus.Entry {
	var out []planstatus.Entry
	for _, t := range tasks {
		if t.Status == "Suggestion" && !IsOldPlansFile(t.Path) {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

var (
	frontmatterBlockRE = regexp.MustCompile(`^---\r?\n(?s:.*?)\r?\n---\r?\n?`)
	scaffoldHeadingRE  = regexp.MustCompile(`(?i)^(goal|context|summary|problem|the problem|plan|overview|proposed changes)$`)
	headingRE          = regexp.MustCompile(`^#+\s+(.*)$`)
	spacesRE           = regexp.MustCompile(`\s+`)
)

// SummarizeSuggestion is a one-line gist of a suggestion file: its first real heading or line.
// "" when the file is unreadable or has nothing to show.
func SummarizeSuggestion(relPath, root string) string {
	b, err := os.ReadFile(filepath.Join(root, "agent_docs", filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	body := string(b)
	if loc := frontmatterBlockRE.FindStringIndex(body); loc != nil {
		body = body[loc[1]:]
	}
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if m := headingRE.FindStringSubmatch(line); m != nil {
			line = m[1]
		}
		gist := strings.TrimSpace(spacesRE.ReplaceAllString(line, " "))
		if gist == "" || scaffoldHeadingRE.MatchString(gist) {
			continue
		}
		if r := []rune(gist); len(r) > 100 {
			return string(r[:99]) + "…"
		}
		return gist
	}
	return ""
}

func suggestionStem(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(p, "plans/agent_suggestions/"), ".md")
}

// FormatSuggestionList is one numbered line per suggestion: filename stem plus gist.
func FormatSuggestionList(suggestions []planstatus.Entry, root string) []string {
	lines := make([]string, len(suggestions))
	for i, s := range suggestions {
		line := fmt.Sprintf("  %2d. %s", i+1, suggestionStem(s.Path))
		if gist := SummarizeSuggestion(s.Path, root); gist != "" {
			line += " — " + gist
		}
		lines[i] = line
	}
	return lines
}

func TaskHasSubtasks(task planstatus.Entry, tasks []Item) bool {
	for _, t := range tasks {
		if t.SubtaskOf == task.Path {
			return true
		}
	}
	return false
}

func IsAutoTask(task planstatus.Entry, tasks []Item) bool {
	if task.Auto {
		return true
	}
	if task.SubtaskOf == "" {
		return false
	}
	for _, t := range tasks {
		if t.Path == task.SubtaskOf {
			return t.Auto
		}
	}
	return false
}

// FormatTaskLine renders one numbered menu row; index is zero-based.
func FormatTaskLine(task Item, index int, tasks []Item, isTTY bool) string {
	prefix := "  "
	if task.IsSubtask {
		prefix = "     ↳ "
	}
	label := ColorizeStatus(task.Status, task.Status, isTTY)
	if task.WorktreeBranch != "" {
		label += " @ " + task.WorktreeBranch
	}
	line := fmt.Sprintf("%s%d) [%s] %s", prefix, index+1, label, task.Path)
	if task.Priority != 0 {
		line += fmt.Sprintf(" [P%d]", task.Priority)
	}
	if IsAutoTask(task.Entry, tasks) {
		line += " [auto]"
	}
	return line
}
