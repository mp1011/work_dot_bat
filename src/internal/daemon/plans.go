// Package daemon is the Auto Daemon: it finds plans marked `auto: true`, works each one in its own
// git worktree via a coding agent, and records the outcome. A pure planner (this file) decides
// which plans and in what order; the runner takes its side effects as injected functions.
package daemon

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"work_dot_bat/src/internal/planclaims"
	"work_dot_bat/src/internal/planstatus"
)

// Plan is a plan as the daemon sees it: where its authoritative copy lives and, for a blocked
// plan, why.
type Plan struct {
	planstatus.Entry
	Source         string // "main" or a worktree path
	BlockedReason  string
	WorktreeBranch string
}

func isResolved(s string) bool        { return s == "Ready for Review" || s == "Done" }
func isSiblingResolved(s string) bool { return isResolved(s) || s == "Cancelled" }

var blockedSectionRE = regexp.MustCompile(`(?is)##\s*Blocked[^\r\n]*\r?\n(.*?)(?:\r?\n##\s|$)`)

// ExtractBlockedReason is the first non-blank line of the "## Blocked…" section, or "".
func ExtractBlockedReason(content string) string {
	m := blockedSectionRE.FindStringSubmatch(content)
	if m == nil {
		return ""
	}
	for _, l := range regexp.MustCompile(`\r?\n`).Split(m[1], -1) {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

func norm(p string) string { return planstatus.NormalizePlanPath(p) }

func find(plans []Plan, path string) *Plan {
	n := norm(path)
	for i := range plans {
		if norm(plans[i].Path) == n {
			return &plans[i]
		}
	}
	return nil
}

// IsAutoCandidate: the plan has auto: true, or an ancestor does.
func IsAutoCandidate(e Plan, plans []Plan) bool {
	if e.Auto {
		return true
	}
	if e.SubtaskOf == "" {
		return false
	}
	visited := map[string]bool{e.Path: true}
	cur := norm(e.SubtaskOf)
	for cur != "" && !visited[cur] {
		visited[cur] = true
		parent := find(plans, cur)
		if parent == nil {
			break
		}
		if parent.Auto {
			return true
		}
		cur = ""
		if parent.SubtaskOf != "" {
			cur = norm(parent.SubtaskOf)
		}
	}
	return false
}

func HasSubtasks(e Plan, plans []Plan) bool {
	n := norm(e.Path)
	for _, p := range plans {
		if p.SubtaskOf != "" && norm(p.SubtaskOf) == n {
			return true
		}
	}
	return false
}

// DaemonBranchFor is the slug of the plan's top-level `auto: true` ancestor, walking subtask_of
// upward; the plan's own slug if it has no such ancestor.
func DaemonBranchFor(planPath string, plans []Plan) string {
	n := norm(planPath)
	entry := find(plans, n)
	if entry == nil || entry.SubtaskOf == "" {
		return planstatus.SlugForPlan(n)
	}
	var chain []*Plan
	visited := map[string]bool{n: true}
	cur := entry
	for cur != nil && cur.SubtaskOf != "" {
		parentPath := norm(cur.SubtaskOf)
		if parentPath == "" || visited[parentPath] {
			break
		}
		visited[parentPath] = true
		parent := find(plans, parentPath)
		if parent == nil {
			break
		}
		chain = append(chain, parent)
		cur = parent
	}
	var top *Plan
	for _, a := range chain {
		if a.Auto {
			top = a // the last auto ancestor is the furthest up
		}
	}
	if top != nil {
		return planstatus.SlugForPlan(top.Path)
	}
	return planstatus.SlugForPlan(n)
}

// ReadCheckoutPlans reads every plan in one checkout (the main repo or a worktree).
func ReadCheckoutPlans(checkoutRoot string) []Plan {
	var plans []Plan
	for _, e := range planstatus.ReadStatus(checkoutRoot) {
		p := Plan{Entry: e}
		if e.Status == "Blocked - User Input Required" {
			if b, err := os.ReadFile(filepath.Join(checkoutRoot, "agent_docs", filepath.FromSlash(e.Path))); err == nil {
				p.BlockedReason = ExtractBlockedReason(planstatus.ParseFrontmatter(string(b)).Body)
			}
		}
		plans = append(plans, p)
	}
	return plans
}

// ReadPlanStates reads main's plans, then each live (non-prunable, existing, non-main) worktree's.
// A plan's authoritative entry is its worktree copy when the plan's daemon branch has a live
// worktree containing the file, else main's. Plans that exist only in a worktree (created by a
// breakdown there) are included.
func ReadPlanStates(repoRoot string, worktrees []planstatus.Worktree, readPlans func(string) []Plan) []Plan {
	if readPlans == nil {
		readPlans = ReadCheckoutPlans
	}
	normRoot := strings.TrimRight(strings.ReplaceAll(abs(repoRoot), `\`, "/"), "/")
	type wtPlans struct {
		wt    planstatus.Worktree
		plans []Plan
	}
	byBranch := map[string]wtPlans{}
	var branchOrder []string
	for _, w := range worktrees {
		if w.Prunable || w.Branch == "" {
			continue
		}
		if strings.TrimRight(strings.ReplaceAll(abs(w.Path), `\`, "/"), "/") == normRoot {
			continue
		}
		if _, err := os.Stat(w.Path); err != nil {
			continue
		}
		plans := readPlans(w.Path)
		for i := range plans {
			plans[i].Source, plans[i].WorktreeBranch = w.Path, w.Branch
		}
		if _, dup := byBranch[w.Branch]; !dup {
			branchOrder = append(branchOrder, w.Branch)
		}
		byBranch[w.Branch] = wtPlans{w, plans}
	}
	mainPlans := readPlans(repoRoot)
	for i := range mainPlans {
		mainPlans[i].Source = "main"
	}
	paths := map[string]bool{}
	for _, p := range mainPlans {
		paths[p.Path] = true
	}
	for _, b := range branchOrder {
		for _, p := range byBranch[b].plans {
			paths[p.Path] = true
		}
	}
	findIn := func(plans []Plan, path string) *Plan {
		for i := range plans {
			if plans[i].Path == path {
				return &plans[i]
			}
		}
		return nil
	}
	var out []Plan
	for path := range paths {
		mainEntry := findIn(mainPlans, path)
		var chosen *Plan
		for _, b := range branchOrder {
			if wtEntry := findIn(byBranch[b].plans, path); wtEntry != nil && DaemonBranchFor(path, byBranch[b].plans) == b {
				chosen = wtEntry
				break
			}
		}
		if chosen == nil && mainEntry != nil {
			chosen = mainEntry
			if data, ok := byBranch[DaemonBranchFor(path, mainPlans)]; ok {
				if wtEntry := findIn(data.plans, path); wtEntry != nil {
					chosen = wtEntry
				}
			}
		}
		if chosen == nil {
			for _, b := range branchOrder {
				if wtEntry := findIn(byBranch[b].plans, path); wtEntry != nil {
					chosen = wtEntry
					break
				}
			}
		}
		if chosen != nil {
			out = append(out, *chosen)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func abs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// Result is the planner's verdict.
type Result struct {
	Eligible []Plan // In Progress first, then Not Started, each in path order
	Blocked  []Plan // auto plans waiting on a human
	Reasons  map[string]string
}

// Inputs are what eligibility needs beyond the plans themselves.
type Inputs struct {
	// Committed reports whether the plan file is git-tracked and clean; nil = everything is.
	Committed func(Plan) bool
	// Claims are the current claims; only live plan-scope claims block a plan.
	Claims []planclaims.Listed
}

func hasLiveClaim(path string, claims []planclaims.Listed) bool {
	for _, c := range claims {
		if c.Scope == "plan" && c.ID == path && c.Decision.State == planclaims.Held {
			return true
		}
	}
	return false
}

// Eligibility applies the rules in order: auto (or auto ancestor); no subtasks of its own; status
// Not Started/In Progress; plan file committed; every earlier sibling resolved; depends_on
// targets Ready for Review or Done; no live claim.
func Eligibility(plans []Plan, in Inputs) Result {
	var eligible, blocked []Plan
	reasons := map[string]string{}
	for _, e := range plans {
		if !IsAutoCandidate(e, plans) {
			reasons[e.Path] = "not marked auto: true"
			continue
		}
		if e.Status == "Blocked - User Input Required" {
			blocked = append(blocked, e)
		}
		if HasSubtasks(e, plans) {
			reasons[e.Path] = "has subtasks — work the subtasks instead"
			continue
		}
		if e.Status != "Not Started" && e.Status != "In Progress" {
			reasons[e.Path] = "status is " + e.Status
			continue
		}
		if in.Committed != nil && !in.Committed(e) {
			reasons[e.Path] = "uncommitted changes in plan file"
			continue
		}
		if e.SubtaskOf != "" {
			parent := norm(e.SubtaskOf)
			var siblings []Plan
			for _, p := range plans {
				if p.SubtaskOf != "" && norm(p.SubtaskOf) == parent && p.Path != e.Path {
					siblings = append(siblings, p)
				}
			}
			sort.Slice(siblings, func(i, j int) bool { return siblings[i].Path < siblings[j].Path })
			var blocker *Plan
			for i := range siblings {
				if siblings[i].Path < e.Path && !isSiblingResolved(siblings[i].Status) {
					blocker = &siblings[i]
					break
				}
			}
			if blocker != nil {
				reasons[e.Path] = "earlier subtask " + blocker.Path + " (" + blocker.Status + ")"
				continue
			}
		}
		if e.DependsOn != "" {
			depReason := ""
			for _, t := range strings.Split(e.DependsOn, ",") {
				t = strings.TrimSpace(t)
				if t == "" {
					continue
				}
				dep := find(plans, t)
				if dep == nil {
					depReason = "depends_on " + t + " (missing)"
					break
				}
				if !isResolved(dep.Status) {
					depReason = "depends_on " + t + " (" + dep.Status + ")"
					break
				}
			}
			if depReason != "" {
				reasons[e.Path] = depReason
				continue
			}
		}
		if hasLiveClaim(e.Path, in.Claims) {
			reasons[e.Path] = "plan is claimed"
			continue
		}
		eligible = append(eligible, e)
	}
	byPath := func(s []Plan) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].Path < s[j].Path })
	}
	var inProgress, notStarted []Plan
	for _, e := range eligible {
		if e.Status == "In Progress" {
			inProgress = append(inProgress, e)
		} else {
			notStarted = append(notStarted, e)
		}
	}
	byPath(inProgress)
	byPath(notStarted)
	byPath(blocked)
	return Result{Eligible: append(inProgress, notStarted...), Blocked: blocked, Reasons: reasons}
}

func sourceLabel(e Plan) string {
	if e.Source == "" || e.Source == "main" {
		return "[main]"
	}
	branch := e.WorktreeBranch
	if branch == "" {
		branch = planstatus.SlugForPlan(e.Source)
	}
	return "[worktree " + branch + "]"
}

// FormatAutoStatusReport is the `auto_status` text: blocked, eligible (next first), not eligible.
func FormatAutoStatusReport(r Result, plans []Plan) string {
	var auto []Plan
	for _, e := range plans {
		if IsAutoCandidate(e, plans) {
			auto = append(auto, e)
		}
	}
	if len(auto) == 0 {
		return `No plans are marked "auto: true".`
	}
	inSet := func(list []Plan) map[string]bool {
		m := map[string]bool{}
		for _, e := range list {
			m[e.Path] = true
		}
		return m
	}
	eligible, blocked := inSet(r.Eligible), inSet(r.Blocked)
	var notEligible []Plan
	for _, e := range auto {
		if !eligible[e.Path] && !blocked[e.Path] {
			notEligible = append(notEligible, e)
		}
	}
	sort.Slice(notEligible, func(i, j int) bool { return notEligible[i].Path < notEligible[j].Path })

	all := append(append(append([]Plan{}, r.Blocked...), r.Eligible...), notEligible...)
	maxPath, maxSrc := 0, 0
	for _, e := range all {
		if n := len([]rune(e.Path)); n > maxPath {
			maxPath = n
		}
		if n := len(sourceLabel(e)); n > maxSrc {
			maxSrc = n
		}
	}
	pad := func(s string, n int) string {
		if d := n - len([]rune(s)); d > 0 {
			return s + strings.Repeat(" ", d)
		}
		return s
	}
	line := func(e Plan, detail string) string {
		return "  " + pad(e.Path, maxPath) + "   " + pad(sourceLabel(e), maxSrc) + "   " + detail
	}
	var sections []string
	if len(r.Blocked) > 0 {
		lines := []string{"BLOCKED — daemon will not proceed (" + itoa(len(r.Blocked)) + "):"}
		for _, e := range r.Blocked {
			d := e.BlockedReason
			if d == "" {
				d = "Blocked - User Input Required"
			}
			lines = append(lines, line(e, d))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if len(r.Eligible) > 0 {
		lines := []string{"ELIGIBLE (next first):"}
		for _, e := range r.Eligible {
			lines = append(lines, line(e, e.Status))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if len(notEligible) > 0 {
		lines := []string{"NOT ELIGIBLE (" + itoa(len(notEligible)) + "):"}
		for _, e := range notEligible {
			d := r.Reasons[e.Path]
			if d == "" {
				d = e.Status
			}
			lines = append(lines, line(e, d))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func itoa(n int) string { return strconv.Itoa(n) }
