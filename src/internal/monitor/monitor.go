package monitor

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
)

// Item represents a single plan entry in the monitor report.
type Item struct {
	Status   string // "Active", "Stalled", "Blocked", "Ready for Review"
	PlanPath string // relative plan path starting with "plans/..."
	Agent    string // agent name (e.g. "agy", "claude", "codex"), empty if unknown
}

// Group represents a project or project worktree and its matching items.
type Group struct {
	Project  string // e.g. "work_dot_bat"
	Worktree string // empty if main branch, else branch name e.g. "feature-auth"
	Items    []Item
}

// Config configures the monitor service.
type Config struct {
	RegistryFile   string
	Projects       []projects.Project
	ProcessLister  ProcessLister
	CollectWT      planstatus.CollectWorktreeStatuses
	Now            func() time.Time
	Stdout         io.Writer
	Stderr         io.Writer
	Colorize       bool
	ProjectFilter  string // optional filter for a specific project name
}

// FormatStatus returns the status string, optionally wrapped in ANSI color codes.
func FormatStatus(status string, colorize bool) string {
	if !colorize {
		return "[" + status + "]"
	}
	// Color codes:
	// Active: 36 (Cyan) or 32 (Green)
	// Stalled: 33 (Yellow)
	// Blocked: 31 (Red)
	// Ready for Review: 32 (Green)
	var code string
	switch status {
	case "Active":
		code = "36" // Cyan
	case "Stalled":
		code = "33" // Yellow
	case "Blocked":
		code = "31" // Red
	case "Ready for Review":
		code = "32" // Green
	default:
		return "[" + status + "]"
	}
	return "\x1b[" + code + "m[" + status + "]\x1b[0m"
}

// FormatHeader renders the group header:
// "<Project>" if on main branch, or "<Project : Worktree>" if in a worktree.
// Following the specification:
// <Project : Worktree>
// - omit "Worktree" if work is done on main branch
func FormatHeader(project, worktree string) string {
	if worktree == "" {
		return project
	}
	return fmt.Sprintf("%s : %s", project, worktree)
}

// FormatItem formats a single plan entry:
// "    [STATUS] [Relative Plan Path] - [Agent]"
// - omit " - [Agent]" if it is not known
func FormatItem(item Item, colorize bool) string {
	statusStr := FormatStatus(item.Status, colorize)
	if item.Agent != "" {
		return fmt.Sprintf("    %s %s - %s", statusStr, item.PlanPath, item.Agent)
	}
	return fmt.Sprintf("    %s %s", statusStr, item.PlanPath)
}

// FormatReport renders the entire monitor output for all groups.
func FormatReport(groups []Group, colorize bool) string {
	var lines []string
	for i, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		if i > 0 && len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, FormatHeader(g.Project, g.Worktree))
		for _, item := range g.Items {
			lines = append(lines, FormatItem(item, colorize))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// NormalizeSlash converts backslashes to forward slashes.
func NormalizeSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}

// DetectAgentFromCmdLine attempts to identify the agent from command-line arguments.
func DetectAgentFromCmdLine(cmd string) string {
	lower := strings.ToLower(cmd)
	switch {
	case strings.Contains(lower, "agy"):
		return "agy"
	case strings.Contains(lower, "claude"):
		return "claude"
	case strings.Contains(lower, "codex"):
		return "codex"
	default:
		return ""
	}
}

// FindActiveProcess checks if any running process matches the given plan path and project/worktree.
func FindActiveProcess(procs []ProcessInfo, planPath, projectPath, worktreePath string) (bool, string) {
	normPlan := strings.ToLower(planstatus.NormalizePlanPath(planPath))
	agentDocsPlan := "agent_docs/" + normPlan
	normProj := strings.ToLower(NormalizeSlash(projectPath))
	normWT := ""
	if worktreePath != "" {
		normWT = strings.ToLower(NormalizeSlash(worktreePath))
	}

	for _, p := range procs {
		cmd := strings.ToLower(NormalizeSlash(p.CommandLine))
		if !strings.Contains(cmd, normPlan) && !strings.Contains(cmd, agentDocsPlan) {
			continue
		}

		// Verify project context if possible:
		// If command line contains a path, check that it matches this project or worktree
		if normWT != "" && strings.Contains(cmd, normWT) {
			return true, DetectAgentFromCmdLine(p.CommandLine)
		}
		if strings.Contains(cmd, normProj) {
			return true, DetectAgentFromCmdLine(p.CommandLine)
		}

		// If the command line does not have directory arguments (e.g. simple `claude "..."`),
		// the unique plan path reference is sufficient.
		return true, DetectAgentFromCmdLine(p.CommandLine)
	}
	return false, ""
}

// FindLatestAgentInLedger returns the agent of the most recent session recorded for planPath.
func FindLatestAgentInLedger(dir, planPath string) string {
	ledgerFile := repo.LedgerPath(dir)
	sessions := agentsession.FindSessionsForPlan(ledgerFile, planPath)
	if len(sessions) == 0 {
		return ""
	}
	last := sessions[len(sessions)-1]
	if agent, ok := last["agent"].(string); ok && agent != "" {
		return agent
	}
	return ""
}

// IsBlockedStatus returns true if the status string indicates a blocked task.
func IsBlockedStatus(status string) bool {
	return status == "Blocked" || status == "Blocked - User Input Required"
}

// CollectProjectGroups gathers status items for a single project.
func CollectProjectGroups(ctx context.Context, p projects.Project, procs []ProcessInfo, collectWT planstatus.CollectWorktreeStatuses) []Group {
	if _, err := os.Stat(p.Path); err != nil {
		return nil
	}

	// 1. Collect worktree status overrides
	wtMap := map[string]planstatus.WorktreeStatus{}
	if collectWT != nil {
		if w, err := collectWT(p.Path); err == nil && w != nil {
			wtMap = w
		}
	}

	// 2. Read main-checkout plan entries
	mainEntries := planstatus.ReadStatus(p.Path)

	// 3. Map each plan to its appropriate worktree/main group
	// Key: worktree branch ("" for main)
	type planInfo struct {
		Path           string
		Status         string
		WorktreeBranch string
		WorktreePath   string
	}

	planByPath := map[string]planInfo{}
	for _, e := range mainEntries {
		info := planInfo{
			Path:   e.Path,
			Status: e.Status,
		}
		if wt, ok := wtMap[e.Path]; ok && !wt.Fossil && wt.Branch != "" {
			info.Status = planstatus.EffectiveStatus(e.Status, wt.Status)
			info.WorktreeBranch = wt.Branch
			info.WorktreePath = wt.Worktree
		}
		planByPath[e.Path] = info
	}

	// Also add plans that exist only in a worktree
	for pPath, wt := range wtMap {
		if wt.Fossil {
			continue
		}
		if _, exists := planByPath[pPath]; !exists {
			planByPath[pPath] = planInfo{
				Path:           pPath,
				Status:         wt.Status,
				WorktreeBranch: wt.Branch,
				WorktreePath:   wt.Worktree,
			}
		}
	}

	// 4. Group plans by WorktreeBranch
	groupsMap := map[string][]Item{}
	worktreeOrder := []string{""} // main first

	// Sort plans for deterministic output
	var sortedPaths []string
	for k := range planByPath {
		sortedPaths = append(sortedPaths, k)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		info := planByPath[path]
		rawStatus := info.Status
		wtBranch := info.WorktreeBranch
		wtDir := info.WorktreePath
		if wtDir == "" {
			wtDir = p.Path
		}

		var finalStatus string
		var agent string

		switch {
		case rawStatus == "In Progress":
			active, detectedAgent := FindActiveProcess(procs, info.Path, p.Path, wtDir)
			if active {
				finalStatus = "Active"
				agent = detectedAgent
			} else {
				finalStatus = "Stalled"
			}
		case IsBlockedStatus(rawStatus):
			finalStatus = "Blocked"
		case rawStatus == "Ready for Review":
			finalStatus = "Ready for Review"
		default:
			// "Other statuses are not given." (Not Started, Done, Cancelled, Suggestion)
			continue
		}

		// If agent was not found from live process, lookup in ledger
		if agent == "" {
			agent = FindLatestAgentInLedger(wtDir, info.Path)
			if agent == "" && wtDir != p.Path {
				agent = FindLatestAgentInLedger(p.Path, info.Path)
			}
		}

		item := Item{
			Status:   finalStatus,
			PlanPath: planstatus.NormalizePlanPath(info.Path),
			Agent:    agent,
		}

		if _, exists := groupsMap[wtBranch]; !exists {
			if wtBranch != "" {
				worktreeOrder = append(worktreeOrder, wtBranch)
			}
		}
		groupsMap[wtBranch] = append(groupsMap[wtBranch], item)
	}

	// 5. Build final list of groups in order
	var result []Group
	for _, wt := range worktreeOrder {
		items := groupsMap[wt]
		if len(items) > 0 {
			result = append(result, Group{
				Project:  p.Name,
				Worktree: wt,
				Items:    items,
			})
		}
	}
	return result
}

// Run executes the universal monitor across registered projects.
func Run(cfg Config) int {
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// Load registry if projects not explicitly provided
	projs := cfg.Projects
	if len(projs) == 0 {
		regFile := cfg.RegistryFile
		if regFile == "" {
			regFile = projects.DefaultRegistryFile()
		}
		list, errs, err := projects.LoadRegistry(regFile)
		if err != nil {
			fmt.Fprintf(stderr, "monitor: %v\n", err)
			return 1
		}
		for _, msg := range errs {
			fmt.Fprintf(stderr, "WARNING: %s\n", msg)
		}
		projs = list
	}

	if cfg.ProjectFilter != "" {
		var filtered []projects.Project
		for _, p := range projs {
			if strings.EqualFold(p.Name, cfg.ProjectFilter) {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(stderr, "monitor: unknown project %q\n", cfg.ProjectFilter)
			return 1
		}
		projs = filtered
	}

	// Collect running processes
	lister := cfg.ProcessLister
	if lister == nil {
		lister = DefaultProcessLister
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	procs, err := lister(ctx)
	if err != nil {
		// Log warning but proceed: without processes, In Progress plans show as Stalled
		fmt.Fprintf(stderr, "WARNING: unable to query process list: %v\n", err)
	}

	collectWT := cfg.CollectWT
	if collectWT == nil {
		collectWT = planstatus.GitCollectWorktreeStatuses
	}

	// Scan all projects
	var allGroups []Group
	for _, p := range projs {
		groups := CollectProjectGroups(ctx, p, procs, collectWT)
		allGroups = append(allGroups, groups...)
	}

	// Format and print report
	output := FormatReport(allGroups, cfg.Colorize)
	if output != "" {
		fmt.Fprint(stdout, output)
	}
	return 0
}
