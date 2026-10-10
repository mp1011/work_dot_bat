// Package worktree creates and provisions git worktrees for plans and reports where a
// checkout sits (the create_worktree tool and `worktree-info`). It shells out to the git
// binary; tests run it against real temp repos.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"work_dot_bat/src/internal/planstatus"
)

var trailingSlashes = regexp.MustCompile(`/+$`)

func toPosix(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// PathFor is the sibling folder `<project name>-<branch>` next to the project, using the
// project's registered name when known (else the repo folder's name). Always forward slashes.
func PathFor(repoRoot, branch, projectName string) string {
	normalized := trailingSlashes.ReplaceAllString(toPosix(repoRoot), "")
	if projectName != "" {
		parent := ""
		if i := strings.LastIndex(normalized, "/"); i > 0 {
			parent = normalized[:i]
		}
		return parent + "/" + projectName + "-" + branch
	}
	return normalized + "-" + branch
}

func FindForBranch(worktrees []planstatus.Worktree, branch string) *planstatus.Worktree {
	for i := range worktrees {
		if worktrees[i].Branch == branch {
			return &worktrees[i]
		}
	}
	return nil
}

type Action string

const (
	Reuse             Action = "reuse"
	PruneRequired     Action = "prune-required"
	Blocked           Action = "blocked"
	AddExistingBranch Action = "add-existing-branch"
	AddNewBranch      Action = "add-new-branch"
)

type Decision struct {
	Action Action
	Path   string
	Reason string
}

// Decide classifies the gathered state. It never proposes a destructive action: a stale
// registration or an unexpected directory is reported, not cleaned up.
func Decide(worktrees []planstatus.Worktree, branch, targetPath string, targetExists, branchExists bool) Decision {
	if existing := FindForBranch(worktrees, branch); existing != nil {
		if existing.Prunable {
			return Decision{PruneRequired, existing.Path, fmt.Sprintf(
				"branch \"%s\" is registered to %s, but that directory no longer exists. Run `git worktree prune` and try again.",
				branch, existing.Path)}
		}
		return Decision{Reuse, existing.Path, ""}
	}
	if targetExists {
		return Decision{Blocked, targetPath, fmt.Sprintf(
			"%s already exists but is not a registered worktree. Move or remove it yourself and try again.", targetPath)}
	}
	if branchExists {
		return Decision{AddExistingBranch, targetPath, ""}
	}
	return Decision{AddNewBranch, targetPath, ""}
}

// RunOpts describes one child process. Dir is always set for work inside a worktree: a step
// that fell back to the default cwd would test or commit against the main checkout while
// still reporting success.
type RunOpts struct {
	Dir   string
	Env   map[string]string
	Shell bool
}

// Runner runs a command with inherited stdio and returns its exit code.
type Runner func(cmd string, args []string, o RunOpts) (int, error)

// RunCommand is the real Runner. Shell runs the joined command line through the platform
// shell; otherwise the program is exec'd directly (Go resolves npm.cmd on Windows).
func RunCommand(cmd string, args []string, o RunOpts) (int, error) {
	var c *exec.Cmd
	if o.Shell {
		line := strings.Join(append([]string{cmd}, args...), " ")
		if isWindows() {
			c = exec.Command("cmd", "/c", line)
		} else {
			c = exec.Command("sh", "-c", line)
		}
	} else {
		c = exec.Command(cmd, args...)
	}
	c.Dir = o.Dir
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if len(o.Env) > 0 {
		c.Env = os.Environ()
		for k, v := range o.Env {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	err := c.Run()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func isWindows() bool { return filepath.Separator == '\\' }

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func ExistingWorktrees(repoRoot string) ([]planstatus.Worktree, error) {
	out, err := gitOutput(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return planstatus.ParseWorktreeList(out), nil
}

func BranchExists(repoRoot, branch string) bool {
	_, err := gitOutput(repoRoot, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

type EnsureResult struct {
	OK      bool
	Path    string
	Created bool
	Action  Action
	Reason  string
}

// Ensure creates the worktree for branch if needed, or reuses the existing one. It never
// deletes anything.
func Ensure(repoRoot, branch, projectName string, run Runner, log func(string)) (EnsureResult, error) {
	worktrees, err := ExistingWorktrees(repoRoot)
	if err != nil {
		return EnsureResult{}, err
	}
	target := PathFor(repoRoot, branch, projectName)
	_, statErr := os.Stat(target)
	d := Decide(worktrees, branch, target, statErr == nil, BranchExists(repoRoot, branch))

	switch d.Action {
	case Reuse:
		log("Worktree already exists — reusing " + d.Path)
		return EnsureResult{OK: true, Path: d.Path, Action: d.Action}, nil
	case PruneRequired, Blocked:
		log("[ERROR] " + d.Reason)
		return EnsureResult{Path: d.Path, Action: d.Action, Reason: d.Reason}, nil
	}

	args := []string{"worktree", "add", d.Path, "-b", branch}
	if d.Action == AddExistingBranch {
		args = []string{"worktree", "add", d.Path, branch}
	}
	log("Creating worktree at " + d.Path + "...")
	code, err := run("git", args, RunOpts{Dir: repoRoot})
	if err != nil || code != 0 {
		reason := fmt.Sprintf("git %s failed with exit code %d", strings.Join(args, " "), code)
		return EnsureResult{Path: d.Path, Action: d.Action, Reason: reason}, nil
	}
	return EnsureResult{OK: true, Path: d.Path, Created: true, Action: d.Action}, nil
}
