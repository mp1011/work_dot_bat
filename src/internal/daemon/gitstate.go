package daemon

import (
	"os/exec"
	"regexp"
	"strings"
	"work_dot_bat/internal/planstatus"
)

// GitFunc runs git in dir and returns stdout.
type GitFunc func(dir string, args ...string) (string, error)

func RealGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func unquote(v string) string {
	if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		return v[1 : len(v)-1]
	}
	return v
}

// ParseGitStatusPaths: porcelain v1 lines are `XY <path>` or `XY <old> -> <new>` (a rename is
// looked up under its destination), with unusual paths quoted.
func ParseGitStatusPaths(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rest := strings.TrimSpace(line[strings.Index(line, " ")+1:])
		if i := strings.Index(rest, " -> "); i != -1 {
			rest = rest[i+4:]
		}
		paths = append(paths, unquote(strings.TrimSpace(rest)))
	}
	return paths
}

var statusLineRE = regexp.MustCompile(`(?i)^status:\s*`)

// IsDiffOnlyStatusChange: the diff touches nothing but `status:` lines.
func IsDiffOnlyStatusChange(diff string) bool {
	if strings.TrimSpace(diff) == "" {
		return false
	}
	hasStatus := false
	for _, line := range regexp.MustCompile(`\r?\n`).Split(diff, -1) {
		if strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "@@ ") {
			continue
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			if statusLineRE.MatchString(strings.TrimSpace(line[1:])) {
				hasStatus = true
			} else {
				return false
			}
		}
	}
	return hasStatus
}

// CommittedPlanPaths: the entries whose plan file is git-tracked and clean, checked against the
// root each entry actually lives in (a plan created and committed in a worktree but never merged
// has no counterpart in main). Two git calls per distinct root. In a worktree a dirty plan file
// still counts when it is In Progress or its diff is only a status line, since the daemon itself
// writes those.
func CommittedPlanPaths(repoRoot string, plans []Plan, git GitFunc) map[string]bool {
	if git == nil {
		git = RealGit
	}
	byRoot := map[string][]Plan{}
	var order []string
	for _, p := range plans {
		root := repoRoot
		if p.Source != "" && p.Source != "main" {
			root = p.Source
		}
		if _, ok := byRoot[root]; !ok {
			order = append(order, root)
		}
		byRoot[root] = append(byRoot[root], p)
	}
	result := map[string]bool{}
	for _, root := range order {
		tracked, err := git(root, "ls-files", "agent_docs/plans")
		if err != nil {
			continue
		}
		status, err := git(root, "status", "--porcelain", "--", "agent_docs/plans")
		if err != nil {
			continue
		}
		trackedSet := map[string]bool{}
		for _, l := range strings.Split(tracked, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				trackedSet[l] = true
			}
		}
		dirty := map[string]bool{}
		for _, p := range ParseGitStatusPaths(status) {
			dirty[p] = true
		}
		isWorktree := root != repoRoot
		for _, p := range byRoot[root] {
			repoPath := "agent_docs/" + p.Path
			switch {
			case !trackedSet[repoPath]:
			case !dirty[repoPath]:
				result[p.Path] = true
			case isWorktree && p.Status == "In Progress":
				result[p.Path] = true
			case isWorktree:
				if diff, err := git(root, "diff", "--", repoPath); err == nil && IsDiffOnlyStatusChange(diff) {
					result[p.Path] = true
				}
			}
		}
	}
	return result
}

func worktreeList(root string) ([]planstatus.Worktree, error) {
	out, err := RealGit(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return planstatus.ParseWorktreeList(out), nil
}
