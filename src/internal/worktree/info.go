package worktree

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/planstatus"
)

type Info struct {
	Worktree     string
	Branch       string
	MainCheckout string
	SameAsMain   bool
}

func normalize(p string) string {
	abs, err := filepath.Abs(filepath.FromSlash(toPosix(p)))
	if err != nil {
		abs = p
	}
	return strings.ToLower(abs)
}

// ResolveInfo: git always lists the main checkout first, so worktrees[0] is main. An empty
// list falls back to the current path so the function stays total.
func ResolveInfo(currentPath, currentBranch string, worktrees []planstatus.Worktree) Info {
	main := currentPath
	if len(worktrees) > 0 {
		main = worktrees[0].Path
	}
	return Info{currentPath, currentBranch, main, normalize(currentPath) == normalize(main)}
}

// FormatInfo renders the label/value table `worktree-info` prints.
func FormatInfo(i Info) string {
	same := "no"
	if i.SameAsMain {
		same = "yes"
	}
	rows := [][2]string{{"worktree", i.Worktree}, {"branch", i.Branch}, {"main checkout", i.MainCheckout}, {"same as main", same}}
	width := 0
	for _, r := range rows {
		width = max(width, len(r[0]))
	}
	lines := make([]string, len(rows))
	for n, r := range rows {
		lines[n] = fmt.Sprintf("%-*s %s", width, r[0], r[1])
	}
	return strings.Join(lines, "\n")
}

// GitFunc runs git in dir and returns trimmed stdout; injected so tests can stub it.
type GitFunc func(dir string, args ...string) (string, error)

func realGit(dir string, args ...string) (string, error) {
	out, err := gitOutput(dir, args...)
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
	}
	return strings.TrimSpace(out), err
}

func GetInfo(cwd string, git GitFunc) (Info, error) {
	if git == nil {
		git = realGit
	}
	current, err := git(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return Info{}, err
	}
	branch, err := git(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return Info{}, err
	}
	list, err := git(cwd, "worktree", "list", "--porcelain")
	if err != nil {
		return Info{}, err
	}
	return ResolveInfo(toPosix(current), branch, planstatus.ParseWorktreeList(list)), nil
}
