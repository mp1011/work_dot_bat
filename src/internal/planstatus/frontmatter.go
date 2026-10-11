// Package planstatus reads and writes the YAML frontmatter block that carries a plan file's
// status under agent_docs/plans/, and implements the plan-status CLI over it. Entry paths are
// relative to agent_docs/, e.g. "plans/foo/bar.md".
package planstatus

import (
	"regexp"
	"strconv"
	"strings"
)

var Statuses = []string{
	"Not Started",
	"In Progress",
	"Blocked - User Input Required",
	"Ready for Review",
	"Done",
	"Cancelled",
}

// RestrictedStatuses may only be set with --human: Cancelled is the human's call.
var RestrictedStatuses = []string{"Cancelled"}

// DerivedStatuses are accepted as a `list` filter but never written to a plan file.
var DerivedStatuses = []string{"Suggestion"}

const SuggestionsPathPrefix = "plans/agent_suggestions/"

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// DeriveEffectiveStatus turns a parked suggestion's on-disk "Not Started" into "Suggestion",
// unless the plan opted into unattended work with auto: true.
func DeriveEffectiveStatus(relPath, rawStatus string, auto bool) string {
	if rawStatus == "Not Started" && strings.HasPrefix(relPath, SuggestionsPathPrefix) && !auto {
		return "Suggestion"
	}
	return rawStatus
}

var frontmatterRE = regexp.MustCompile(`^---\r?\n((?s:.*?))\r?\n---\r?\n?`)
var leadingNewlinesRE = regexp.MustCompile(`^\r?\n+`)
var quoteEdgesRE = regexp.MustCompile(`^["']|["']$`)

// NormalizePlanPath trims quotes and whitespace, uses forward slashes, and drops an
// agent_docs/ prefix. "" means no path.
func NormalizePlanPath(p string) string {
	if p == "" {
		return ""
	}
	cleaned := quoteEdgesRE.ReplaceAllString(strings.TrimSpace(p), "")
	cleaned = strings.ReplaceAll(cleaned, `\`, "/")
	return strings.TrimPrefix(cleaned, "agent_docs/")
}

var priorityRE = regexp.MustCompile(`^[pP]?([1-5])$`)

// ParsePriority accepts P1..P5 or 1..5; 0 means unprioritised.
func ParsePriority(raw string) int {
	m := priorityRE.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// Frontmatter is the parsed block. Status is nil when the file has no status field.
type Frontmatter struct {
	Status        *string
	Informational bool
	Auto          bool
	Priority      int
	SubtaskOf     string
	DependsOn     string
	BlockedBy     []string
	Verification  string // "manual" or ""
	Body          string
}

func keyRE(key string) *regexp.Regexp { return regexp.MustCompile(`^` + key + `:\s*`) }

func fieldValue(lines []string, key string) (string, bool) {
	re := keyRE(key)
	for _, l := range lines {
		if re.MatchString(l) {
			return strings.TrimSpace(re.ReplaceAllString(l, "")), true
		}
	}
	return "", false
}

func parseBlockedBy(lines []string) []string {
	re := keyRE("blocked_by")
	idx := -1
	for i, l := range lines {
		if re.MatchString(l) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil
	}
	val := strings.TrimSpace(re.ReplaceAllString(lines[idx], ""))
	var result []string
	add := func(s string) {
		if n := NormalizePlanPath(s); n != "" {
			result = append(result, n)
		}
	}
	if val != "" {
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
			for _, item := range strings.Split(val[1:len(val)-1], ",") {
				add(item)
			}
		} else {
			add(val)
		}
		return result
	}
	bullet := regexp.MustCompile(`^\s*-\s+`)
	indented := regexp.MustCompile(`^\s+`)
	for _, l := range lines[idx+1:] {
		if bullet.MatchString(l) {
			add(strings.TrimSpace(bullet.ReplaceAllString(l, "")))
		} else if strings.TrimSpace(l) == "" || indented.MatchString(l) {
			continue
		} else {
			break
		}
	}
	return result
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func ParseFrontmatter(content string) Frontmatter {
	m := frontmatterRE.FindStringSubmatch(content)
	if m == nil {
		return Frontmatter{Body: content}
	}
	lines := splitLines(m[1])
	fm := Frontmatter{Body: content[len(m[0]):]}
	if v, ok := fieldValue(lines, "status"); ok {
		fm.Status = &v
	}
	trueLine := func(key string) bool {
		re := regexp.MustCompile(`^` + key + `:\s*true\s*$`)
		for _, l := range lines {
			if re.MatchString(l) {
				return true
			}
		}
		return false
	}
	fm.Informational = trueLine("informational")
	fm.Auto = trueLine("auto")
	if v, ok := fieldValue(lines, "priority"); ok {
		fm.Priority = ParsePriority(v)
	}
	if v, ok := fieldValue(lines, "subtask_of"); ok {
		fm.SubtaskOf = NormalizePlanPath(v)
	}
	if v, ok := fieldValue(lines, "depends_on"); ok {
		fm.DependsOn = NormalizePlanPath(v)
	}
	fm.BlockedBy = parseBlockedBy(lines)
	if v, ok := fieldValue(lines, "verification"); ok && v == "manual" {
		fm.Verification = "manual"
	}
	return fm
}

// splitBlock returns the block's non-blank lines whose key is not in drop, and the body.
func splitBlock(content string, drop ...string) (other []string, body string) {
	m := frontmatterRE.FindStringSubmatch(content)
	if m == nil {
		return nil, content
	}
	for _, l := range splitLines(m[1]) {
		if strings.TrimSpace(l) == "" {
			continue
		}
		dropped := false
		for _, k := range drop {
			if keyRE(k).MatchString(l) {
				dropped = true
				break
			}
		}
		if !dropped {
			other = append(other, l)
		}
	}
	return other, content[len(m[0]):]
}

func assemble(block []string, body string) string {
	return strings.Join(block, "\n") + "\n\n" + leadingNewlinesRE.ReplaceAllString(body, "")
}

// SetFrontmatterStatus replaces or adds status:, preserving every other field.
func SetFrontmatterStatus(content, status string) string {
	other, body := splitBlock(content, "status")
	block := append(append([]string{"---", "status: " + status}, other...), "---")
	return assemble(block, body)
}

// SetFrontmatterVerification adds, replaces, or (when "") removes verification:.
func SetFrontmatterVerification(content, verification string) string {
	other, body := splitBlock(content, "verification")
	block := append([]string{"---"}, other...)
	if verification != "" {
		block = append(block, "verification: "+verification)
	}
	return assemble(append(block, "---"), body)
}

func SetFrontmatterSubtask(content, status, subtaskOf string) string {
	other, body := splitBlock(content, "status", "subtask_of")
	block := append(append([]string{"---", "status: " + status, "subtask_of: " + subtaskOf}, other...), "---")
	return assemble(block, body)
}

// HasManualVerificationSteps reports whether a "## Manual Verification" heading has at
// least one non-blank line before the next "##" heading.
func HasManualVerificationSteps(body string) bool {
	lines := splitLines(body)
	h := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "## Manual Verification" {
			h = i
			break
		}
	}
	if h == -1 {
		return false
	}
	heading := regexp.MustCompile(`^##\s`)
	for _, l := range lines[h+1:] {
		if heading.MatchString(l) {
			break
		}
		if strings.TrimSpace(l) != "" {
			return true
		}
	}
	return false
}

// SetFrontmatterBody keeps the frontmatter block and replaces everything after it.
func SetFrontmatterBody(content, body string) string {
	other, _ := splitBlock(content)
	if other == nil {
		return body
	}
	return assemble(append(append([]string{"---"}, other...), "---"), body)
}
