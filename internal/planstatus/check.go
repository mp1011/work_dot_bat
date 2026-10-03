package planstatus

import (
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

var statusRankTable = map[string]int{
	"Not Started":                   0,
	"In Progress":                   1,
	"Blocked - User Input Required": 2,
	"Ready for Review":              3,
	"Done":                          4,
}

func statusRank(s string) int {
	if r, ok := statusRankTable[s]; ok {
		return r
	}
	return -1
}

// WorktreeStatus is one worktree branch's view of a plan file. Collecting these from git is
// the worktree subtask's job; this package only consumes them.
type WorktreeStatus struct {
	Status            string
	Branch            string
	Worktree          string
	Fossil            bool
	CommitsBehind     int
	BranchTouchedPlan bool
	SubtaskOf         string
	DependsOn         string
	Auto              bool
	Priority          int
}

// CollectWorktreeStatuses gathers every live worktree's view, keyed by plan path.
type CollectWorktreeStatuses func(root string) (map[string]WorktreeStatus, error)

// EffectiveStatus is the furthest-along of the main and worktree views; a worktree never
// moves a plan backward or into Cancelled.
func EffectiveStatus(mainStatus, worktreeStatus string) string {
	if mainStatus == "Cancelled" || mainStatus == "Done" {
		return mainStatus
	}
	if worktreeStatus == "" || worktreeStatus == "Cancelled" {
		return mainStatus
	}
	if statusRank(worktreeStatus) > statusRank(mainStatus) {
		return worktreeStatus
	}
	return mainStatus
}

// ApplyWorktreeStatuses overlays worktree statuses onto main-checkout entries and appends
// plans that exist only in a worktree.
func ApplyWorktreeStatuses(entries []Entry, byPath map[string]WorktreeStatus) []Entry {
	seen := map[string]bool{}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		seen[e.Path] = true
		found, ok := byPath[e.Path]
		if !ok || found.Fossil {
			out = append(out, e)
			continue
		}
		if status := EffectiveStatus(e.Status, found.Status); status != e.Status {
			e.Status, e.WorktreeBranch, e.WorktreePath = status, found.Branch, found.Worktree
		}
		out = append(out, e)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		found := byPath[p]
		if seen[p] || found.Fossil {
			continue
		}
		out = append(out, Entry{Path: p, Status: found.Status, WorktreeBranch: found.Branch, WorktreePath: found.Worktree,
			SubtaskOf: found.SubtaskOf, DependsOn: found.DependsOn, Auto: found.Auto, Priority: found.Priority})
	}
	return out
}

// FileStatus is the raw frontmatter view of one git-tracked plan file, for `check`.
type FileStatus struct {
	Path          string
	Status        *string
	Informational bool
	BlockedBy     []string
	SubtaskOf     string
}

func (f FileStatus) status() string {
	if f.Status == nil {
		return ""
	}
	return *f.Status
}

type PathError struct{ Path, Error string }
type InvalidStatus struct{ Path, Status string }
type Divergence struct {
	Path, MainStatus, Branch, BranchStatus string
	CommitsBehind                          int
}
type DoneParent struct{ Parent, Subtask, SubtaskStatus string }
type StaleBlocked struct {
	Path string
	Days int
}

type CheckResult struct {
	OK                         bool
	MissingFrontmatter         []string
	InvalidStatus              []InvalidStatus
	BlockedByErrors            []PathError
	SubtaskErrors              []PathError
	WorktreeDivergence         []Divergence
	OpenSubtasksForDoneParents []DoneParent
	StaleBlockedPlans          []StaleBlocked
}

func ClassifyFrontmatterStatus(files []FileStatus) CheckResult {
	var r CheckResult
	var tracked []FileStatus
	byPath := map[string]FileStatus{}
	for _, f := range files {
		byPath[f.Path] = f
		if !f.Informational {
			tracked = append(tracked, f)
		}
	}
	for _, f := range tracked {
		if f.Status == nil {
			r.MissingFrontmatter = append(r.MissingFrontmatter, f.Path)
		}
	}
	sort.Strings(r.MissingFrontmatter)
	for _, f := range files {
		if f.Status != nil && !contains(Statuses, *f.Status) {
			r.InvalidStatus = append(r.InvalidStatus, InvalidStatus{f.Path, *f.Status})
		}
	}
	sort.Slice(r.InvalidStatus, func(i, j int) bool { return r.InvalidStatus[i].Path < r.InvalidStatus[j].Path })

	for _, f := range files {
		for _, b := range f.BlockedBy {
			target, ok := byPath[b]
			if !ok {
				r.BlockedByErrors = append(r.BlockedByErrors, PathError{f.Path, `lists unknown plan "` + b + `" in blocked_by`})
			} else if (f.status() == "Done" || f.status() == "Ready for Review") && target.status() != "Done" {
				r.BlockedByErrors = append(r.BlockedByErrors, PathError{f.Path,
					`is marked "` + f.status() + `" but is blocked by "` + b + `" which is "` + target.status() + `"`})
			}
		}
	}

	paths := make([]string, 0, len(tracked))
	for _, f := range tracked {
		paths = append(paths, f.Path)
	}
	folders := planMdFolders(paths)
	for _, f := range tracked {
		if path.Base(f.Path) == "plan.md" {
			continue
		}
		dir := path.Dir(f.Path)
		if folders[dir] && NormalizePlanPath(f.SubtaskOf) != dir+"/plan.md" {
			r.SubtaskErrors = append(r.SubtaskErrors, PathError{f.Path,
				`is in a folder with plan.md but missing or incorrect subtask_of (expected "` + dir + `/plan.md")`})
		}
	}
	sort.Slice(r.SubtaskErrors, func(i, j int) bool { return r.SubtaskErrors[i].Path < r.SubtaskErrors[j].Path })

	r.OK = len(r.MissingFrontmatter) == 0 && len(r.InvalidStatus) == 0 && len(r.BlockedByErrors) == 0 && len(r.SubtaskErrors) == 0
	return r
}

func resolved(s string) bool { return s == "Done" || s == "Cancelled" }

// ClassifyWorktreeDivergence reports plans where a worktree branch and main genuinely disagree.
func ClassifyWorktreeDivergence(files []FileStatus, byPath map[string]WorktreeStatus) []Divergence {
	main := map[string]string{}
	for _, f := range files {
		main[f.Path] = f.status()
	}
	var out []Divergence
	for p, wt := range byPath {
		ms, ok := main[p]
		if !ok || ms == "" {
			continue
		}
		d := Divergence{p, ms, wt.Branch, wt.Status, wt.CommitsBehind}
		if resolved(ms) && !resolved(wt.Status) && wt.BranchTouchedPlan {
			out = append(out, d)
		} else if !resolved(ms) && wt.Fossil && statusRank(wt.Status) > statusRank(ms) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ClassifyDoneParentsWithOpenSubtasks flags Done parents whose subtasks are still open
// (worktree-authoritative).
func ClassifyDoneParentsWithOpenSubtasks(files []FileStatus, byPath map[string]WorktreeStatus) []DoneParent {
	var entries []Entry
	for _, f := range files {
		if f.Informational || f.Status == nil {
			continue
		}
		entries = append(entries, Entry{Path: f.Path, Status: *f.Status, SubtaskOf: f.SubtaskOf})
	}
	overlaid := ApplyWorktreeStatuses(entries, byPath)
	idx := map[string]Entry{}
	for _, e := range overlaid {
		if n := NormalizePlanPath(e.Path); n != "" {
			idx[n] = e
		}
	}
	var out []DoneParent
	for _, child := range overlaid {
		if child.SubtaskOf == "" {
			continue
		}
		parent, ok := idx[NormalizePlanPath(child.SubtaskOf)]
		if ok && parent.Status == "Done" && !resolved(child.Status) {
			out = append(out, DoneParent{parent.Path, child.Path, child.Status})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Parent != out[j].Parent {
			return out[i].Parent < out[j].Parent
		}
		return out[i].Subtask < out[j].Subtask
	})
	return out
}

const DefaultStaleBlockedDays = 14

// CheckOptions injects the git- and clock-dependent parts so tests need no real repo.
type CheckOptions struct {
	ListPlanFiles           func(root string) ([]string, error)
	CollectWorktreeStatuses CollectWorktreeStatuses
	StaleDays               int
	Now                     time.Time
	FileCommitTime          func(root, rel string) (time.Time, bool)
	HasWorkingTreeChanges   func(root, rel string) bool
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	return string(out), err
}

// PlanPathsFromGitOutput keeps the plans/**/*.md lines outside the archive, agent_docs-relative.
func PlanPathsFromGitOutput(output string) []string {
	var out []string
	for _, f := range strings.Split(output, "\n") {
		f = strings.TrimRight(f, "\r")
		if strings.HasPrefix(f, "agent_docs/plans/") && strings.HasSuffix(f, ".md") && !archived(strings.TrimPrefix(f, "agent_docs/")) {
			out = append(out, strings.TrimPrefix(f, "agent_docs/"))
		}
	}
	return out
}

func gitListPlanFiles(root string) ([]string, error) {
	out, err := gitOut(root, "ls-files", "agent_docs/plans")
	if err != nil {
		return nil, err
	}
	return PlanPathsFromGitOutput(out), nil
}

func gitFileCommitTime(root, rel string) (time.Time, bool) {
	out, err := gitOut(root, "log", "-1", "--format=%ct", "--", "agent_docs/"+NormalizePlanPath(rel))
	out = strings.TrimSpace(out)
	if err != nil || out == "" {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

func gitHasWorkingTreeChanges(root, rel string) bool {
	out, err := gitOut(root, "status", "--porcelain", "--", "agent_docs/"+NormalizePlanPath(rel))
	return err == nil && strings.TrimSpace(out) != ""
}

// FindStaleBlockedPlans lists Blocked plans untouched (committed and in the working tree) for
// longer than the threshold.
func FindStaleBlockedPlans(files []FileStatus, root string, o CheckOptions) []StaleBlocked {
	threshold := time.Duration(o.StaleDays) * 24 * time.Hour
	var out []StaleBlocked
	for _, f := range files {
		if f.Informational || f.status() != "Blocked - User Input Required" || o.HasWorkingTreeChanges(root, f.Path) {
			continue
		}
		t, ok := o.FileCommitTime(root, f.Path)
		if !ok {
			continue
		}
		if age := o.Now.Sub(t); age > threshold {
			out = append(out, StaleBlocked{f.Path, int(age / (24 * time.Hour))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Check verifies every git-tracked plan file has valid frontmatter, no worktree branch
// silently disagrees with main, and no Done parent has open subtasks.
func Check(root string, o CheckOptions) (CheckResult, error) {
	if o.ListPlanFiles == nil {
		o.ListPlanFiles = gitListPlanFiles
	}
	if o.StaleDays == 0 {
		o.StaleDays = DefaultStaleBlockedDays
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.FileCommitTime == nil {
		o.FileCommitTime = gitFileCommitTime
	}
	if o.HasWorkingTreeChanges == nil {
		o.HasWorkingTreeChanges = gitHasWorkingTreeChanges
	}
	paths, err := o.ListPlanFiles(root)
	if err != nil {
		return CheckResult{}, err
	}
	files := make([]FileStatus, 0, len(paths))
	for _, rel := range paths {
		content, ok := readFile(root, rel)
		if !ok {
			files = append(files, FileStatus{Path: rel})
			continue
		}
		fm := ParseFrontmatter(content)
		files = append(files, FileStatus{Path: rel, Status: fm.Status, Informational: fm.Informational,
			BlockedBy: fm.BlockedBy, SubtaskOf: fm.SubtaskOf})
	}
	r := ClassifyFrontmatterStatus(files)

	worktrees := map[string]WorktreeStatus{}
	if o.CollectWorktreeStatuses != nil {
		if w, err := o.CollectWorktreeStatuses(root); err == nil && w != nil {
			worktrees = w
		}
	}
	r.WorktreeDivergence = ClassifyWorktreeDivergence(files, worktrees)
	r.OpenSubtasksForDoneParents = ClassifyDoneParentsWithOpenSubtasks(files, worktrees)
	r.StaleBlockedPlans = FindStaleBlockedPlans(files, root, o)
	r.OK = r.OK && len(r.WorktreeDivergence) == 0 && len(r.OpenSubtasksForDoneParents) == 0
	return r, nil
}
