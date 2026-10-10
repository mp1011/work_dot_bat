package work

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"work_dot_bat/src/internal/planstatus"
)

// IsStatusOnlyPath reports whether a path only ever reflects plan-status bookkeeping (plan
// markdown under agent_docs/plans/ and the generated plan_status.js shim), never agent work.
func IsStatusOnlyPath(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	p = strings.TrimPrefix(strings.TrimSuffix(p, `"`), `"`)
	return strings.HasPrefix(p, "agent_docs/plans/") || p == "agent_docs/plan_status.js"
}

var (
	goalRE        = regexp.MustCompile(`(?im)^#{1,2}\s+GOAL\b(.*)$`)
	nextHeadingRE = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	inlineTrimRE  = regexp.MustCompile(`^[:\s-]+`)
)

// HasNonEmptyGoal reports whether the plan has a # GOAL section with any content.
func HasNonEmptyGoal(content string) bool {
	if content == "" {
		return false
	}
	loc := goalRE.FindStringSubmatchIndex(content)
	if loc == nil {
		return false
	}
	inline := ""
	if loc[3] > loc[2] {
		inline = strings.TrimSpace(inlineTrimRE.ReplaceAllString(content[loc[2]:loc[3]], ""))
	}
	after := content[loc[1]:]
	if h := nextHeadingRE.FindStringIndex(after); h != nil {
		after = after[:h[0]]
	}
	return strings.TrimSpace(inline+" "+after) != ""
}

var porcelainRE = regexp.MustCompile(`^([ ?MADRCU!]{2})\s+(.+)$`)

// Breakdown is the verdict on whether a change set is a pure plan breakdown into new subtasks.
type Breakdown struct {
	IsBreakdown bool
	Subtasks    []string
	Reason      string
}

type entry struct{ code, rawPath string }

// PlanBreakdownInfo classifies porcelain lines (or bare paths): at least one new subtask .md,
// each with subtask_of pointing at one parent and a non-empty # GOAL; the only modified plan
// is that parent; and nothing but plan bookkeeping files changed. parentPlan may be "".
func PlanBreakdownInfo(lines []string, readFile func(string) (string, error), parentPlan string) Breakdown {
	var entries []entry
	for _, l := range lines {
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		code, p := "", l
		if m := porcelainRE.FindStringSubmatch(l); m != nil {
			code, p = m[1], m[2]
			if i := strings.Index(p, " -> "); i != -1 {
				p = p[i+4:]
			}
		}
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(strings.TrimSuffix(p, `"`), `"`)
		entries = append(entries, entry{code, strings.ReplaceAll(p, `\`, "/")})
	}
	if len(entries) == 0 {
		return Breakdown{Reason: "empty change list"}
	}
	for _, e := range entries {
		if !IsStatusOnlyPath(e.rawPath) {
			return Breakdown{Reason: "non-status path: " + e.rawPath}
		}
	}
	for _, e := range entries {
		if strings.Contains(e.code, "D") {
			return Breakdown{Reason: "deleted file: " + e.rawPath}
		}
	}
	expected := ""
	if parentPlan != "" {
		expected = planstatus.NormalizePlanPath(parentPlan)
	}
	var added, modified []entry
	for _, e := range entries {
		if e.rawPath == "agent_docs/plan_status.js" || e.rawPath == "plan_status.js" {
			continue
		}
		norm := planstatus.NormalizePlanPath(e.rawPath)
		switch {
		case e.code != "":
			if e.code == "??" || strings.Contains(e.code, "A") {
				added = append(added, e)
			} else if strings.Contains(e.code, "M") {
				modified = append(modified, e)
			}
		case expected != "" && norm == expected:
			modified = append(modified, e)
		case strings.HasSuffix(e.rawPath, ".md"):
			added = append(added, e)
		}
	}
	if len(added) == 0 {
		return Breakdown{Reason: "no added subtask files"}
	}
	var subtasks []string
	for _, e := range added {
		if !strings.HasSuffix(e.rawPath, ".md") {
			return Breakdown{Reason: e.rawPath + " is not a markdown file"}
		}
		content, err := readFile(e.rawPath)
		if err != nil {
			content, err = readFile(planstatus.NormalizePlanPath(e.rawPath))
			if err != nil {
				return Breakdown{Reason: "cannot read " + e.rawPath}
			}
		}
		fm := planstatus.ParseFrontmatter(content)
		if fm.SubtaskOf == "" {
			return Breakdown{Reason: e.rawPath + " missing subtask_of"}
		}
		if !HasNonEmptyGoal(content) {
			return Breakdown{Reason: e.rawPath + " has empty goal"}
		}
		parent := planstatus.NormalizePlanPath(fm.SubtaskOf)
		if expected == "" {
			expected = parent
		} else if parent != expected {
			return Breakdown{Reason: fmt.Sprintf("%s subtask_of (%s) does not match expected parent (%s)", e.rawPath, parent, expected)}
		}
		subtasks = append(subtasks, e.rawPath)
	}
	for _, e := range modified {
		if expected != "" && planstatus.NormalizePlanPath(e.rawPath) != expected {
			return Breakdown{Reason: fmt.Sprintf("modified unrelated plan file: %s (expected %s)", e.rawPath, expected)}
		}
	}
	return Breakdown{IsBreakdown: true, Subtasks: subtasks}
}

// StatusFunc returns `git status --porcelain` output for a directory.
type StatusFunc func(dir string) (string, error)

func GitStatusPorcelain(dir string) (string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func fileReader(dir string) func(string) (string, error) {
	return func(p string) (string, error) {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			b, err = os.ReadFile(p)
		}
		return string(b), err
	}
}

// DetectPlanBreakdown classifies the working tree in dir. A git failure is "not a breakdown".
func DetectPlanBreakdown(dir, parentPlan string, status StatusFunc, readFile func(string) (string, error)) Breakdown {
	if status == nil {
		status = GitStatusPorcelain
	}
	out, err := status(dir)
	if err != nil {
		return Breakdown{}
	}
	if readFile == nil {
		readFile = fileReader(dir)
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return PlanBreakdownInfo(lines, readFile, parentPlan)
}

// TreeHasNonStatusChanges is true when the tree has a change that is not plan bookkeeping, or
// when the bookkeeping changes are a valid breakdown into new subtasks. An unknowable tree
// (git error) is true: a possibly-good run must not be discarded.
func TreeHasNonStatusChanges(dir, parentPlan string, status StatusFunc, readFile func(string) (string, error)) bool {
	if status == nil {
		status = GitStatusPorcelain
	}
	out, err := status(dir)
	if err != nil {
		return true
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if len(l) > 3 {
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		p := l[3:]
		if i := strings.Index(p, " -> "); i != -1 {
			p = p[i+4:]
		}
		if !IsStatusOnlyPath(strings.TrimSpace(p)) {
			return true
		}
	}
	if readFile == nil {
		readFile = fileReader(dir)
	}
	return PlanBreakdownInfo(lines, readFile, parentPlan).IsBreakdown
}
