package agentsession

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FormatDuration is "Nm Ns" / "Ns", or "N/A" when either end is missing or unparseable.
func FormatDuration(startedAt, endedAt string) string {
	if startedAt == "" || endedAt == "" {
		return "N/A"
	}
	start, err1 := time.Parse(time.RFC3339Nano, startedAt)
	end, err2 := time.Parse(time.RFC3339Nano, endedAt)
	if err1 != nil || err2 != nil || end.Before(start) {
		return "N/A"
	}
	total := int(end.Sub(start) / time.Second)
	if mins := total / 60; mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, total%60)
	}
	return fmt.Sprintf("%ds", total)
}

// Analysis is what a session's log shows the agent did.
type Analysis struct {
	FilesRead, FilesEdited, CommandsRun []string
	StepZeroBlocked, Escalated          bool
}

var (
	// The Node patterns use a back-reference to pair the optional quote; Go has none, so each
	// quote style is an alternative, tried in the same order.
	readRE = regexp.MustCompile(`(?i)(?:view_file.*?AbsolutePath[:\s]+|Reading file\s+)(?:"([^"\r\n\s]+)"|'([^'\r\n\s]+)'|([^'"\r\n\s]+))`)
	editRE = regexp.MustCompile(`(?i)(?:replace_file_content|write_to_file).*?TargetFile[:\s]+(?:"([^"\r\n\s]+)"|'([^'\r\n\s]+)'|([^'"\r\n\s]+))`)
	cmdRE  = regexp.MustCompile(`(?i)(?:run_command.*?CommandLine[:\s]+|Running command:\s+)(?:"([^\r\n]+?)"|'([^\r\n]+?)'|([^\r\n]+?))$`)

	blockedRE    = regexp.MustCompile(`(?i)BLOCKED`)
	escalationRE = regexp.MustCompile(`(?i)testability_escalation`)
	needsInputRE = regexp.MustCompile(`(?i)Blocked\s*—\s*Needs User Input`)
	lineSplitRE  = regexp.MustCompile(`\r?\n`)
)

func firstGroup(m []string) string {
	for _, g := range m[1:] {
		if g != "" {
			return g
		}
	}
	return ""
}

// ParseLogContent extracts files read and edited (each once, in first-seen order), commands run
// and the two step-zero/escalation flags from an agent log.
func ParseLogContent(content string) Analysis {
	var a Analysis
	seenRead, seenEdit := map[string]bool{}, map[string]bool{}
	for _, line := range lineSplitRE.Split(content, -1) {
		if m := readRE.FindStringSubmatch(line); m != nil {
			if f := firstGroup(m); f != "" && !seenRead[f] {
				seenRead[f] = true
				a.FilesRead = append(a.FilesRead, f)
			}
		}
		if m := editRE.FindStringSubmatch(line); m != nil {
			if f := firstGroup(m); f != "" && !seenEdit[f] {
				seenEdit[f] = true
				a.FilesEdited = append(a.FilesEdited, f)
			}
		}
		if m := cmdRE.FindStringSubmatch(line); m != nil {
			if c := strings.TrimSpace(firstGroup(m)); c != "" {
				a.CommandsRun = append(a.CommandsRun, c)
			}
		}
	}
	a.StepZeroBlocked = blockedRE.MatchString(content)
	a.Escalated = escalationRE.MatchString(content) || needsInputRE.MatchString(content)
	return a
}

func orNA(s string) string {
	if s == "" {
		return "N/A"
	}
	return s
}

// numberText prints a JSON number the way JavaScript would (integers without a fraction).
func numberText(v any) string {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return fmt.Sprintf("%d", int64(n))
		}
		return fmt.Sprint(n)
	}
	return fmt.Sprint(v)
}

func str(e Entry, k string) string { s, _ := e[k].(string); return s }

// FormatSessionReport is the report `analyze_session` prints for one ledger row.
func FormatSessionReport(s Entry, a Analysis) string {
	exit := ""
	if v, ok := s["exitCode"]; ok && v != nil {
		exit = " (Exit Code: " + numberText(v) + ")"
	}
	status := str(s, "status")
	if status == "" {
		status = "unknown"
	}
	list := func(title string, items []string) []string {
		out := []string{fmt.Sprintf("%s (%d):", title, len(items))}
		if len(items) == 0 {
			return append(out, "  (none detected)")
		}
		for _, it := range items {
			out = append(out, "  - "+it)
		}
		return out
	}
	yes := func(b bool) string {
		if b {
			return "YES"
		}
		return "no"
	}
	lines := []string{
		"===================================================",
		"Session:    " + str(s, "sessionName"),
		"Plan:       " + orNA(str(s, "plan")),
		"Agent:      " + orNA(str(s, "agent")),
		"Status:     " + status + exit,
		fmt.Sprintf("Duration:   %s (%s -> %s)", FormatDuration(str(s, "startedAt"), str(s, "endedAt")), orNA(str(s, "startedAt")), orNA(str(s, "endedAt"))),
		"Session ID: " + orNA(str(s, "sessionId")),
		"Log File:   " + orNA(str(s, "logFile")),
		"---------------------------------------------------",
	}
	lines = append(lines, list("Files Read", a.FilesRead)...)
	lines = append(lines, list("Files Edited", a.FilesEdited)...)
	lines = append(lines, list("Commands Run", a.CommandsRun)...)
	lines = append(lines,
		"---------------------------------------------------",
		"Step Zero Blocked:      "+yes(a.StepZeroBlocked),
		"Testability Escalation: "+yes(a.Escalated),
		"===================================================",
	)
	return strings.Join(lines, "\n")
}

// ReclassifyLedger re-labels rows recorded `completed` that had really bounced off a usage limit
// (the claude CLI exits 0 on one). Only completed rows with a session id are looked up. classify
// decides where transcripts live. It changes rows in place and returns the names it changed.
func ReclassifyLedger(rows []*Row, classify func(*Row) Verdict) (changed []string) {
	for _, r := range rows {
		if r.Str("status") != "completed" || r.Str("sessionId") == "" {
			continue
		}
		v := classify(r)
		if v.Status != RateLimited {
			continue
		}
		changed = append(changed, r.Str("sessionName"))
		r.Set("status", "rate-limited")
		if v.ResetAt != nil {
			r.Set("resetAt", ISO(*v.ResetAt))
		}
	}
	return changed
}

// ReclassifyOrphans re-labels rows left `in-progress` for longer than maxAge as
// `abandoned`, stamping endedAt with now. A row with an unparseable start counts as ancient.
func ReclassifyOrphans(rows []*Row, now time.Time, maxAge time.Duration) (changed []string) {
	for _, r := range rows {
		if r.Str("status") != "in-progress" {
			continue
		}
		if started, err := time.Parse(time.RFC3339Nano, r.Str("startedAt")); err == nil && now.Sub(started) < maxAge {
			continue
		}
		changed = append(changed, r.Str("sessionName"))
		r.Set("status", "abandoned")
		r.Set("endedAt", ISO(now))
	}
	return changed
}
