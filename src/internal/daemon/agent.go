package daemon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/repo"
)

// Outcome is what an agent in daemon mode reports on its last marker line.
type Outcome struct {
	Kind   string // "done" | "subtasks" | "blocked"
	Reason string // blocked only
}

var (
	outcomeRE = regexp.MustCompile("(?i)^(?:\\*\\*)?(?:`)?AUTO_OUTCOME:\\s*(.+?)(?:`)?(?:\\*\\*)?$")
	doneRE    = regexp.MustCompile(`(?i)^done\b`)
	subRE     = regexp.MustCompile(`(?i)^subtasks\b`)
	blockedRE = regexp.MustCompile(`(?i)^blocked\b`)
	reasonRE  = regexp.MustCompile(`(?i)^blocked\s*(?:[—–\-:]\s*)?(.*)$`)
	newlineRE = regexp.MustCompile(`\r?\n`)
)

// ParseOutcome reads `AUTO_OUTCOME: done|subtasks|blocked — <reason>`. Only the LAST marker line
// counts; nil when there is none.
func ParseOutcome(text string) *Outcome {
	var last *Outcome
	for _, line := range newlineRE.Split(text, -1) {
		m := outcomeRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		payload := strings.TrimSpace(m[1])
		switch {
		case doneRE.MatchString(payload):
			last = &Outcome{Kind: "done"}
		case subRE.MatchString(payload):
			last = &Outcome{Kind: "subtasks"}
		case blockedRE.MatchString(payload):
			o := &Outcome{Kind: "blocked"}
			if r := reasonRE.FindStringSubmatch(payload); r != nil {
				o.Reason = strings.TrimSpace(r[1])
			}
			last = o
		}
	}
	return last
}

// Assessment is the verdict of an `/assess-done` run.
type Assessment struct {
	Complete bool
	Gaps     []string
}

var (
	assessRE   = regexp.MustCompile("(?i)^(?:\\*\\*)?(?:`)?ASSESSMENT:\\s*(complete|incomplete)(?:`)?(?:\\*\\*)?$")
	assessAny  = regexp.MustCompile("(?i)^(?:\\*\\*)?(?:`)?ASSESSMENT:")
	bulletRE   = regexp.MustCompile(`^[-*]\s+.+`)
	numberedRE = regexp.MustCompile(`^\d+\.\s+.+`)
	bulletText = regexp.MustCompile(`^[-*]\s+(.+)$`)
	numText    = regexp.MustCompile(`^\d+\.\s+(.+)$`)
)

func isBullet(l string) bool { return bulletRE.MatchString(l) || numberedRE.MatchString(l) }

func bulletBody(l string) string {
	if m := bulletText.FindStringSubmatch(l); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := numText.FindStringSubmatch(l); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(l)
}

// ParseAssessment reads `ASSESSMENT: complete|incomplete`; for incomplete, the bullet list just
// above the marker is the list of gaps. Only the LAST marker counts; nil when none.
func ParseAssessment(text string) *Assessment {
	lines := newlineRE.Split(text, -1)
	var last *Assessment
	for idx, raw := range lines {
		m := assessRE.FindStringSubmatch(strings.TrimSpace(raw))
		if m == nil {
			continue
		}
		if strings.EqualFold(m[1], "complete") {
			last = &Assessment{Complete: true, Gaps: []string{}}
			continue
		}
		gaps := []string{}
		i, nonBullet, found := idx-1, 0, false
		for i >= 0 {
			l := strings.TrimSpace(lines[i])
			if isBullet(l) {
				found = true
				break
			}
			if strings.HasPrefix(l, "#") || assessAny.MatchString(l) {
				break
			}
			if l != "" {
				nonBullet++
				if nonBullet > 3 {
					break
				}
			}
			i--
		}
		if found {
			for i >= 0 {
				cur := strings.TrimSpace(lines[i])
				if cur == "" {
					peek := i - 1
					for peek >= 0 && strings.TrimSpace(lines[peek]) == "" {
						peek--
					}
					if peek >= 0 && isBullet(strings.TrimSpace(lines[peek])) {
						i--
						continue
					}
					break
				}
				if !isBullet(cur) {
					break
				}
				gaps = append([]string{bulletBody(cur)}, gaps...)
				i--
			}
		}
		last = &Assessment{Complete: false, Gaps: gaps}
	}
	return last
}

const outcomeFooter = "Finish with exactly one final line:\nAUTO_OUTCOME: done\nAUTO_OUTCOME: subtasks\nAUTO_OUTCOME: blocked — <one-line reason>"

func BuildDaemonWorkPrompt(planPath, daemonDoc string) string {
	return strings.Join([]string{
		"/do-work-yolo " + planPath,
		"",
		"DAEMON MODE",
		"The daemon manages branches, git commits, and review status.",
		"Skip Step Zero entirely. Do not run plan-status.sh.",
		"Do not commit or push.",
		"Follow the DAEMON MODE instructions in " + daemonDoc + ".",
		outcomeFooter,
	}, "\n")
}

// Problem is why the previous attempt needs a follow-up. Exactly one field is meaningful.
type Problem struct {
	Message   string   // free text; "no-changes" selects the no-code-changes wording
	Failures  []string // failing checks
	Gaps      []string // assessment gaps
	NoChanges bool
}

func (p Problem) isZero() bool {
	return p.Message == "" && len(p.Failures) == 0 && len(p.Gaps) == 0 && !p.NoChanges
}

func (p Problem) lines() []string {
	noChanges := func() []string {
		return []string{"Problem: The previous attempt completed without producing any code changes.",
			"You must implement the actual changes required by the plan."}
	}
	switch {
	case p.NoChanges, strings.Contains(strings.ToLower(p.Message), "no code changes"), strings.ToLower(p.Message) == "no-changes":
		return noChanges()
	case len(p.Failures) > 0:
		out := []string{"Automated test/check failures:"}
		for _, f := range p.Failures {
			out = append(out, "- "+f)
		}
		return append(out, "", "Investigate the failure(s), re-run the check(s) to view full error output, and fix them.")
	case len(p.Gaps) > 0:
		out := []string{"Assessment identified incomplete requirements (gaps):"}
		for _, g := range p.Gaps {
			out = append(out, "- "+g)
		}
		return append(out, "", "Address each gap so all plan requirements are fully satisfied.")
	case p.Message != "":
		return []string{"Problem: " + p.Message}
	}
	return nil
}

// BuildDaemonFollowupPrompt is the prompt after failing checks, no code changes, or gaps.
func BuildDaemonFollowupPrompt(planPath string, p Problem, attempt, maxAttempts int) string {
	lines := []string{fmt.Sprintf("Follow-up on %s (attempt %d of %d).", planPath, attempt, maxAttempts), "", "DAEMON MODE", ""}
	lines = append(lines, p.lines()...)
	lines = append(lines, "", "Do not commit or push. Do not run plan-status.sh.",
		"Finish with exactly one final line:", "AUTO_OUTCOME: done | subtasks | blocked — <reason>")
	return strings.Join(lines, "\n")
}

// BuildDaemonContinuationPrompt resumes work after a usage-limit pause or an interrupted
// session, when the worktree holds the agent's own uncommitted work. reason is "interrupted" or
// anything else for a usage limit.
func BuildDaemonContinuationPrompt(planPath, daemonDoc string, p Problem, attempt, maxAttempts int, reason string) string {
	why := "You were interrupted by a usage limit;"
	if reason == "interrupted" {
		why = "You are resuming an earlier session that was interrupted;"
	}
	first := "/do-work-yolo " + planPath
	if attempt > 1 {
		first = fmt.Sprintf("Follow-up on %s (attempt %d of %d).", planPath, attempt, maxAttempts)
	}
	lines := []string{first, "", "DAEMON MODE", why + " the uncommitted changes in this worktree are your own earlier work — continue.", ""}
	lines = append(lines, p.lines()...)
	lines = append(lines, "",
		"The daemon manages branches, git commits, and review status.",
		"Skip Step Zero entirely. Do not run plan-status.sh.",
		"Do not commit or push.",
		"Follow the DAEMON MODE instructions in "+daemonDoc+".",
		outcomeFooter)
	return strings.Join(lines, "\n")
}

func BuildAssessPrompt(planPath string) string { return "/assess-done " + planPath }

// ReadLogFileTail is the last maxBytes of a log file ("" if unreadable).
func ReadLogFileTail(logFile string, maxBytes int64) string {
	if logFile == "" {
		return ""
	}
	f, err := os.Open(logFile)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	if st.Size() > maxBytes {
		if _, err := f.Seek(st.Size()-maxBytes, io.SeekStart); err != nil {
			return ""
		}
	}
	b, _ := io.ReadAll(f)
	return string(b)
}

const logTailBytes = 64 * 1024

var autoOutcomeAny = regexp.MustCompile(`(?i)AUTO_OUTCOME:`)

// ReadFinalMessage is the agent's last message: from the Claude transcript, or for agy from the
// log (when it carries the marker), the Antigravity brain transcript, or the log tail.
func ReadFinalMessage(agent, sessionID, logFile string, dirs agentsession.Dirs, root string) string {
	switch agent {
	case "claude", "c":
		projects := dirs.Projects
		if projects == "" {
			home, _ := os.UserHomeDir()
			projects = filepath.Join(home, ".claude", "projects")
		}
		if t := agentsession.FindTranscriptPath(sessionID, projects); t != "" {
			return agentsession.ReadLastAssistantMessage(t)
		}
		return ""
	case "agy", "a":
		brain := dirs.AgyBrain
		if brain == "" {
			home, _ := os.UserHomeDir()
			brain = filepath.Join(home, ".gemini", "antigravity-cli", "brain")
		}
		target := logFile
		if target == "" && sessionID != "" {
			target = filepath.Join(repo.StateDirName, "logs", sessionID+".log")
		}
		if target != "" {
			if _, err := os.Stat(target); err != nil {
				return ""
			}
			tail := ReadLogFileTail(target, logTailBytes)
			if autoOutcomeAny.MatchString(tail) {
				return tail
			}
			if id := agentsession.ExtractAgyConversationID(tail); id != "" {
				if t := agentsession.FindAgyTranscriptPath(id, brain); t != "" {
					if msg := agentsession.ReadLastAgyAssistantMessage(t); msg != "" {
						return msg
					}
				}
			}
			return tail
		}
		if sessionID != "" {
			if t := agentsession.FindAgyTranscriptPath(sessionID, brain); t != "" {
				return agentsession.ReadLastAgyAssistantMessage(t)
			}
			return ""
		}
		if t := agentsession.FindLatestAgyTranscriptPath(brain); t != "" {
			return agentsession.ReadLastAgyAssistantMessage(t)
		}
		return ""
	}
	if logFile != "" {
		return ReadLogFileTail(logFile, logTailBytes)
	}
	return ""
}
