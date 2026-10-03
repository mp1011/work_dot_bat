package checks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	skillFrontmatterRE = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---`)
	skillNameRE        = regexp.MustCompile(`(?m)^name:\s*(.+)$`)
	skillDescRE        = regexp.MustCompile(`(?m)^description:\s*(.+)$`)
	agentDocsRefRE     = regexp.MustCompile(`agent_docs[/\\][A-Za-z0-9_./\\]+\.md`)
)

// SkillFrontmatter is a skill's name and description ("" when missing).
func SkillFrontmatter(content string) (name, description string) {
	m := skillFrontmatterRE.FindStringSubmatch(content)
	if m == nil {
		return "", ""
	}
	if n := skillNameRE.FindStringSubmatch(m[1]); n != nil {
		name = strings.TrimSpace(n[1])
	}
	if d := skillDescRE.FindStringSubmatch(m[1]); d != nil {
		description = strings.TrimSpace(d[1])
	}
	return
}

// FindAgentDocsRefs are the distinct agent_docs/… .md paths a skill mentions, in order.
func FindAgentDocsRefs(content string) []string {
	var refs []string
	seen := map[string]bool{}
	for _, m := range agentDocsRefRE.FindAllString(content, -1) {
		m = strings.ReplaceAll(m, `\`, "/")
		if !seen[m] {
			seen[m] = true
			refs = append(refs, m)
		}
	}
	return refs
}

func NormalizeSkillName(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "_", "-")
}

type Skill struct {
	Tree, DirName, FilePath, Name, Description string
	Refs                                       []string
}

// localeLess approximates String.localeCompare: case-insensitive first, then lowercase before upper.
func localeLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a > b
}

func ScanSkillTree(root, tree string) []Skill {
	dir := filepath.Join(root, filepath.FromSlash(tree))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		file := filepath.Join(dir, e.Name(), "SKILL.md")
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, file)
		name, desc := SkillFrontmatter(string(b))
		skills = append(skills, Skill{Tree: tree, DirName: e.Name(), FilePath: filepath.ToSlash(rel), Name: name, Description: desc, Refs: FindAgentDocsRefs(string(b))})
	}
	sort.SliceStable(skills, func(i, j int) bool { return localeLess(skills[i].DirName, skills[j].DirName) })
	return skills
}

type SkillViolation struct{ File, Reason string }

type SkillsResult struct {
	Passed                   bool
	Violations               []SkillViolation
	AgentsCount, ClaudeCount int
}

func primaryRef(s Skill) (string, bool) {
	for _, r := range s.Refs {
		if strings.Contains(r, "instructions/") {
			return r, true
		}
	}
	if len(s.Refs) > 0 {
		return s.Refs[0], true
	}
	return "", false
}

// CheckSkills verifies skill frontmatter, that referenced instruction files exist, and that a skill
// present in both trees routes to the same primary instruction.
func CheckSkills(root string) SkillsResult {
	agents := ScanSkillTree(root, ".agents/skills")
	claude := ScanSkillTree(root, ".claude/skills")
	var v []SkillViolation
	for _, s := range append(append([]Skill{}, agents...), claude...) {
		switch {
		case s.Name == "":
			v = append(v, SkillViolation{s.FilePath, `Missing "name:" field in frontmatter`})
		case NormalizeSkillName(s.Name) != NormalizeSkillName(s.DirName):
			v = append(v, SkillViolation{s.FilePath, fmt.Sprintf(`Frontmatter name "%s" does not match directory name "%s"`, s.Name, s.DirName)})
		}
		if s.Description == "" {
			v = append(v, SkillViolation{s.FilePath, `Missing or empty "description:" field in frontmatter`})
		}
		for _, ref := range s.Refs {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ref))); err != nil {
				v = append(v, SkillViolation{s.FilePath, "Referenced file does not exist: " + ref})
			}
		}
	}
	claudeBy := map[string]Skill{}
	for _, s := range claude {
		claudeBy[NormalizeSkillName(s.DirName)] = s
	}
	seen := map[string]bool{}
	for _, a := range agents {
		norm := NormalizeSkillName(a.DirName)
		c, ok := claudeBy[norm]
		if !ok || seen[norm] {
			continue
		}
		seen[norm] = true
		ar, aok := primaryRef(a)
		cr, cok := primaryRef(c)
		if ar != cr || aok != cok {
			show := func(r string, ok bool) string {
				if !ok {
					return "null"
				}
				return r
			}
			v = append(v, SkillViolation{c.FilePath, fmt.Sprintf(`Cross-tree routing mismatch for skill "%s": .agents points to "%s", but .claude points to "%s"`, norm, show(ar, aok), show(cr, cok))})
		}
	}
	return SkillsResult{Passed: len(v) == 0, Violations: v, AgentsCount: len(agents), ClaudeCount: len(claude)}
}

// SkillsMain is `check-skills [check]`.
func SkillsMain(root string, argv []string, out, errOut io.Writer) int {
	if len(argv) > 0 && argv[0] != "check" {
		fmt.Fprintf(errOut, "Unknown command: \"%s\"\nUsage: check-skills [check]\n", argv[0])
		return 1
	}
	r := CheckSkills(root)
	if r.Passed {
		fmt.Fprintf(out, "Skill tree integrity check passed (%d .agents, %d .claude skills scanned).\n", r.AgentsCount, r.ClaudeCount)
		return 0
	}
	fmt.Fprintf(out, "Skill tree integrity check found %d violation(s):\n", len(r.Violations))
	for _, v := range r.Violations {
		fmt.Fprintf(out, "  ❌ %s: %s\n", v.File, v.Reason)
	}
	return 1
}
