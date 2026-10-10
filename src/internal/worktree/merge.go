package worktree

import (
	"fmt"
	"strings"

	"work_dot_bat/src/internal/planstatus"
)

// Availability is the verdict on the "Merge back into main" option. Reason is the whole
// user-facing contract of the gate, so a vague one cannot send anyone hunting for a problem
// that is not there.
type Availability struct {
	Available bool
	Reason    string
}

// MergeAvailability gates merge-back. Order matters: a dirty checkout is reported first because
// it is the thing the user must fix regardless of what else is also true. worktree is nil when
// the branch has none.
func MergeAvailability(repoDirty bool, worktree *planstatus.Worktree, branch, currentBranch string) Availability {
	switch {
	case repoDirty:
		return Availability{Reason: "commit or stash your changes first"}
	case worktree == nil:
		return Availability{Reason: fmt.Sprintf("no worktree for branch %q", branch)}
	case worktree.Prunable:
		return Availability{Reason: fmt.Sprintf("worktree for %q is a stale registration — run `git worktree prune`", branch)}
	case currentBranch != "main":
		return Availability{Reason: fmt.Sprintf("main is not checked out (currently on %q)", currentBranch)}
	}
	return Availability{Available: true}
}

func MergeMenuLabel(a Availability) string {
	if a.Available {
		return "Merge back into main"
	}
	return "[Merge back into main — " + a.Reason + "]"
}

type Commit struct{ SHA, Subject string }

// ParseCommitList splits `git log --oneline` output into sha and subject.
func ParseCommitList(out string) []Commit {
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i := strings.Index(line, " "); i == -1 {
			commits = append(commits, Commit{SHA: line})
		} else {
			commits = append(commits, Commit{SHA: line[:i], Subject: line[i+1:]})
		}
	}
	return commits
}

// GetIncompleteTasksForBranch is the plans sharing branch that are still In Progress or Not Started.
func GetIncompleteTasksForBranch(branch string, all []planstatus.Entry) []planstatus.Entry {
	if branch == "" {
		return nil
	}
	var out []planstatus.Entry
	for _, t := range all {
		if planstatus.BranchNameForPlan(t.Path, all) != branch {
			continue
		}
		if t.Status == "In Progress" || t.Status == "Not Started" {
			out = append(out, t)
		}
	}
	return out
}

func (g GitFunc) or() GitFunc {
	if g == nil {
		return realGit
	}
	return g
}

func lines(out string) []string {
	var res []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			res = append(res, l)
		}
	}
	return res
}

// IsTreeDirty: an unreadable tree is not something to merge from, so an error reads as dirty.
func IsTreeDirty(git GitFunc, tree string) bool {
	out, err := git.or()(tree, "status", "--porcelain")
	return err != nil || strings.TrimSpace(out) != ""
}

// CurrentBranch is "" when it cannot be read.
func CurrentBranch(git GitFunc, tree string) string {
	out, err := git.or()(tree, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// CommitsToMerge is what `git merge <branch>` would bring in. Subtasks share their parent's
// branch, so this is the only place the user sees they are taking more than the one plan they
// selected; an empty result aborts rather than creating an empty merge commit.
func CommitsToMerge(git GitFunc, repoRoot, branch string) []Commit {
	out, err := git.or()(repoRoot, "log", "--oneline", "main.."+branch)
	if err != nil {
		return nil
	}
	return ParseCommitList(out)
}

func ConflictedPaths(git GitFunc, repoRoot string) []string {
	out, err := git.or()(repoRoot, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil
	}
	return lines(out)
}

func StagedFiles(git GitFunc, tree string) []string {
	out, err := git.or()(tree, "diff", "--cached", "--name-only")
	if err != nil {
		return nil
	}
	return lines(out)
}

// IsBranchMerged: true once a merge is committed, or when main already has the same code
// (`git diff main...branch` is empty).
func IsBranchMerged(git GitFunc, repoRoot, branch string) bool {
	g := git.or()
	merged, err := g(repoRoot, "branch", "--merged", "main", "--format=%(refname:short)")
	if err != nil {
		return false
	}
	for _, l := range lines(merged) {
		if l == branch {
			return true
		}
	}
	diff, err := g(repoRoot, "diff", "--name-only", "main..."+branch)
	return err == nil && strings.TrimSpace(diff) == ""
}

type MergeResult struct {
	Outcome   string // "staged" | "conflict" | "failed"
	Conflicts []string
	Staged    []string
}

// MergeBranchIntoMain merges --no-ff --no-commit so the branch lands as one identifiable merge
// and stops with everything staged for review. Nothing here ever commits. A conflict is left in
// progress: resolving or aborting for the user would be wrong or throw their work away.
func MergeBranchIntoMain(git GitFunc, run Runner, repoRoot, branch string) MergeResult {
	code, err := run("git", []string{"merge", "--no-ff", "--no-commit", branch}, RunOpts{Dir: repoRoot})
	if err == nil && code == 0 {
		return MergeResult{Outcome: "staged", Staged: StagedFiles(git, repoRoot)}
	}
	if conflicts := ConflictedPaths(git, repoRoot); len(conflicts) > 0 {
		return MergeResult{Outcome: "conflict", Conflicts: conflicts}
	}
	return MergeResult{Outcome: "failed"}
}

type CleanupResult struct {
	WorktreeRemoved, BranchDeleted bool
	Reason                         string
}

// RemoveWorktreeAndBranch uses safe deletes only — never -D, never --force. If git refuses, the
// refusal is information, not an obstacle to bulldoze.
func RemoveWorktreeAndBranch(run Runner, repoRoot, worktreePath, branch string) CleanupResult {
	opts := RunOpts{Dir: repoRoot}
	if code, err := run("git", []string{"worktree", "remove", worktreePath}, opts); err != nil || code != 0 {
		return CleanupResult{Reason: fmt.Sprintf("Could not remove %s. If a VS Code window or dev server still has it open, "+
			"close them, then run `git worktree prune`.", worktreePath)}
	}
	if code, err := run("git", []string{"branch", "-d", branch}, opts); err != nil || code != 0 {
		return CleanupResult{WorktreeRemoved: true,
			Reason: fmt.Sprintf("Worktree removed, but git refused to delete branch %q. Left it alone.", branch)}
	}
	return CleanupResult{WorktreeRemoved: true, BranchDeleted: true}
}

// CommitWorktree commits whatever an agent left in a worktree so merge-back has something to
// merge. Bounded: worktree branch only, never main, never a push. Returns "committed",
// "no-changes" or "failed".
func CommitWorktree(run Runner, worktreePath, msg string) string {
	opts := RunOpts{Dir: worktreePath}
	ok := func(args ...string) bool {
		code, err := run("git", args, opts)
		return err == nil && code == 0
	}
	if !ok("add", "-A") {
		return "failed"
	}
	// `git diff --cached --quiet` exits 0 when nothing is staged: no changes at all, which is a
	// failure rather than a silent success.
	if ok("diff", "--cached", "--quiet") {
		return "no-changes"
	}
	if !strings.HasPrefix(msg, "auto:") {
		msg = "auto: " + msg
	}
	if ok("commit", "-m", msg) {
		return "committed"
	}
	return "failed"
}
