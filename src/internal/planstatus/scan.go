package planstatus

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var ArchivePrefixes = []string{"plans/done/", "plans/old_plans/"}

func archived(rel string) bool {
	for _, p := range ArchivePrefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// Entry is one tracked plan file. Worktree* are set by ApplyWorktreeStatuses.
type Entry struct {
	Path           string   `json:"path"`
	Status         string   `json:"status"`
	SubtaskOf      string   `json:"subtask_of,omitempty"`
	DependsOn      string   `json:"depends_on,omitempty"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	PlanOrder      []string `json:"plan_order,omitempty"`
	Auto           bool     `json:"auto,omitempty"`
	Priority       int      `json:"priority,omitempty"`
	Verification   string   `json:"verification,omitempty"`
	WorktreeBranch string   `json:"-"`
	WorktreePath   string   `json:"-"`
}

func docsPath(root, rel string) string {
	return filepath.Join(root, "agent_docs", filepath.FromSlash(rel))
}

// WalkPlanFiles lists agent_docs-relative *.md paths under agent_docs/plans, sorted,
// excluding the archive directories.
func WalkPlanFiles(root string) []string {
	var results []string
	agentDocs := filepath.Join(root, "agent_docs")
	filepath.WalkDir(filepath.Join(agentDocs, "plans"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(agentDocs, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if archived(rel + "/") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") && !archived(rel) {
			results = append(results, rel)
		}
		return nil
	})
	sort.Strings(results)
	return results
}

func readFile(root, rel string) (string, bool) {
	b, err := os.ReadFile(docsPath(root, rel))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// ReadStatus returns every plan file on disk that carries a status.
func ReadStatus(root string) []Entry {
	entries := []Entry{}
	for _, rel := range WalkPlanFiles(root) {
		content, ok := readFile(root, rel)
		if !ok {
			continue
		}
		fm := ParseFrontmatter(content)
		if fm.Status == nil {
			continue
		}
		entries = append(entries, Entry{
			Path:         rel,
			Status:       DeriveEffectiveStatus(rel, *fm.Status, fm.Auto),
			SubtaskOf:    fm.SubtaskOf,
			DependsOn:    fm.DependsOn,
			BlockedBy:    fm.BlockedBy,
			PlanOrder:    fm.PlanOrder,
			Auto:         fm.Auto,
			Priority:     fm.Priority,
			Verification: fm.Verification,
		})
	}
	return entries
}

const shimHeader = "// GENERATED from plan file frontmatter by scripts/work_dot_bat/plan-status.sh — do not edit by hand.\n"

func GenerateShim(entries []Entry) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(entries)
	return shimHeader + "window.planData = " + strings.TrimRight(buf.String(), "\n") + ";\n"
}

// WriteShim regenerates agent_docs/plan_status.js, but only in repos that have the optional
// agent_docs/plan_index.html dashboard. Reports whether it wrote.
func WriteShim(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "agent_docs", "plan_index.html")); err != nil {
		return false
	}
	os.WriteFile(filepath.Join(root, "agent_docs", "plan_status.js"), []byte(GenerateShim(ReadStatus(root))), 0o644)
	return true
}

func planMdFolders(files []string) map[string]bool {
	set := map[string]bool{}
	for _, p := range files {
		if path.Base(p) == "plan.md" {
			set[path.Dir(p)] = true
		}
	}
	return set
}
