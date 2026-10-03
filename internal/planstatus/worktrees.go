package planstatus

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	nonSlugRE     = regexp.MustCompile(`[^a-z0-9._-]+`)
	dashRunRE     = regexp.MustCompile(`-+`)
	edgeDashRE    = regexp.MustCompile(`^-|-$`)
	mdSuffixRE    = regexp.MustCompile(`(?i)\.md$`)
	trailingSlash = regexp.MustCompile(`/+$`)
)

func toPosix(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// SlugForPlan is a plan's own name: its file name, except a folder index plan.md, where the
// folder identifies the work.
func SlugForPlan(planPath string) string {
	normalized := trailingSlash.ReplaceAllString(toPosix(planPath), "")
	var segments []string
	for _, s := range strings.Split(normalized, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	base := ""
	if len(segments) > 0 {
		base = segments[len(segments)-1]
		segments = segments[:len(segments)-1]
	}
	base = mdSuffixRE.ReplaceAllString(base, "")
	raw := base
	if strings.ToLower(strings.TrimSpace(base)) == "plan" && len(segments) > 0 {
		raw = segments[len(segments)-1]
	}
	raw = nonSlugRE.ReplaceAllString(strings.ToLower(raw), "-")
	raw = dashRunRE.ReplaceAllString(raw, "-")
	return edgeDashRE.ReplaceAllString(raw, "")
}

// BranchNameForPlan names a subtask's branch after its parent, so every subtask of one
// breakdown shares a branch; anything else is named after itself.
func BranchNameForPlan(planPath string, entries []Entry) string {
	normalized := strings.TrimPrefix(toPosix(planPath), "agent_docs/")
	var entry *Entry
	for i := range entries {
		if entries[i].Path == normalized {
			entry = &entries[i]
			break
		}
	}
	if entry == nil || entry.SubtaskOf == "" {
		return SlugForPlan(normalized)
	}
	if entry.Auto {
		parentAuto := false
		for _, e := range entries {
			if e.Path == entry.SubtaskOf {
				parentAuto = e.Auto
			}
		}
		if !parentAuto {
			return SlugForPlan(normalized)
		}
	}
	return SlugForPlan(entry.SubtaskOf)
}

type Worktree struct {
	Path     string
	Branch   string
	Prunable bool
}

// ParseWorktreeList parses `git worktree list --porcelain`.
func ParseWorktreeList(porcelain string) []Worktree {
	var out []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, raw := range strings.Split(porcelain, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: toPosix(strings.TrimSpace(strings.TrimPrefix(line, "worktree ")))}
		case cur != nil && strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "branch ")), "refs/heads/")
		case cur != nil && strings.HasPrefix(line, "prunable"):
			cur.Prunable = true
		}
	}
	flush()
	return out
}

// A branch further behind main than this is a fossil, not work in progress.
const FossilBranchCommitThreshold = 25

type BranchDivergence struct {
	CommitsBehind      int
	MainChangedPlans   map[string]bool
	BranchChangedPlans map[string]bool
}

// GitRunner runs git in dir and returns stdout; injected so divergence is testable.
type GitRunner func(dir string, args ...string) (string, error)

func defaultGit(dir string, args ...string) (string, error) { return gitOut(dir, args...) }

func CollectBranchDivergence(repoRoot, branch string, git GitRunner) BranchDivergence {
	empty := BranchDivergence{MainChangedPlans: map[string]bool{}, BranchChangedPlans: map[string]bool{}}
	if branch == "" {
		return empty
	}
	lines := func(args ...string) (map[string]bool, error) {
		out, err := git(repoRoot, args...)
		if err != nil {
			return nil, err
		}
		set := map[string]bool{}
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				set[strings.TrimPrefix(l, "agent_docs/")] = true
			}
		}
		return set, nil
	}
	behind, err := git(repoRoot, "rev-list", "--count", branch+"..main")
	if err != nil {
		return empty
	}
	n, _ := strconv.Atoi(strings.TrimSpace(behind))
	mainChanged, err := lines("diff", "--name-only", branch+"...main", "--", "agent_docs/plans")
	if err != nil {
		return empty
	}
	branchChanged, err := lines("diff", "--name-only", "main..."+branch, "--", "agent_docs/plans")
	if err != nil {
		return empty
	}
	return BranchDivergence{n, mainChanged, branchChanged}
}

// IsFossilPlanStatus: is the branch's status for planPath a stale echo of what main used to say?
func IsFossilPlanStatus(planPath string, d BranchDivergence) bool {
	mainTouched, branchTouched := d.MainChangedPlans[planPath], d.BranchChangedPlans[planPath]
	if mainTouched && !branchTouched {
		return true
	}
	return mainTouched && branchTouched && d.CommitsBehind > FossilBranchCommitThreshold
}

// GitCollectWorktreeStatuses is the real CollectWorktreeStatuses: every non-main worktree's
// view of plan status keyed by plan path (a non-fossil view beats a fossil one, then the
// furthest-along status wins).
func GitCollectWorktreeStatuses(repoRoot string) (map[string]WorktreeStatus, error) {
	return CollectWorktreeStatusesWith(repoRoot, defaultGit)
}

func CollectWorktreeStatusesWith(repoRoot string, git GitRunner) (map[string]WorktreeStatus, error) {
	byPath := map[string]WorktreeStatus{}
	out, err := git(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return byPath, nil
	}
	mainRoot := strings.ToLower(absPath(repoRoot))
	for _, wt := range ParseWorktreeList(out) {
		if wt.Prunable {
			continue
		}
		resolved := absPath(wt.Path)
		if strings.ToLower(resolved) == mainRoot {
			continue
		}
		if _, err := os.Stat(filepath.Join(resolved, "agent_docs", "plans")); err != nil {
			continue
		}
		d := CollectBranchDivergence(repoRoot, wt.Branch, git)
		for _, e := range ReadStatus(resolved) {
			cand := WorktreeStatus{
				Status: e.Status, Branch: wt.Branch, Worktree: wt.Path,
				Fossil: IsFossilPlanStatus(e.Path, d), CommitsBehind: d.CommitsBehind,
				BranchTouchedPlan: d.BranchChangedPlans[e.Path],
				SubtaskOf:         e.SubtaskOf, DependsOn: e.DependsOn, Auto: e.Auto, Priority: e.Priority,
			}
			prev, ok := byPath[e.Path]
			if !ok || (prev.Fossil && !cand.Fossil) || (prev.Fossil == cand.Fossil && statusRank(cand.Status) > statusRank(prev.Status)) {
				byPath[e.Path] = cand
			}
		}
	}
	return byPath, nil
}

func absPath(p string) string {
	if a, err := filepath.Abs(filepath.FromSlash(p)); err == nil {
		return a
	}
	return p
}
