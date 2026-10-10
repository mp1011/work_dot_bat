package agentsession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	// The embedded zone database: Windows has no system one for time.LoadLocation.
	_ "time/tzdata"
)

const (
	RateLimitFallbackPause = 30 * time.Minute
	rateLimitBuffer        = time.Minute
	// A bounce is the whole of the session's output; a long summary that merely quotes the
	// limit text must not read as one.
	maxLimitMessageLength = 300
)

var (
	rateLimitRE = regexp.MustCompile(`(?i)hit your (session|usage) limit`)
	resetRE     = regexp.MustCompile(`(?i)resets\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)\s*\(([^)]+)\)`)
)

// ParseRateLimitReset is the next occurrence, after now, of the "resets 1:30pm
// (America/New_York)" wall-clock time in text. ok is false when there is no reset time or the
// zone is unknown; callers then use a fixed back-off.
func ParseRateLimitReset(text string, now time.Time) (time.Time, bool) {
	m := resetRE.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(m[4])
	if err != nil {
		return time.Time{}, false
	}
	hour, _ := strconv.Atoi(m[1])
	hour %= 12
	if strings.EqualFold(m[3], "pm") {
		hour += 12
	}
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	local := now.In(loc)
	candidate := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if candidate.After(now) {
		return candidate, true
	}
	return time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, loc), true
}

func isLimitMessage(text string) bool {
	t := strings.TrimSpace(text)
	return t != "" && utf8.RuneCountInString(t) <= maxLimitMessageLength && rateLimitRE.MatchString(t)
}

type Status string

const (
	Completed   Status = "completed"
	Failed      Status = "failed"
	RateLimited Status = "rate-limited"
)

// Verdict is how an agent invocation ended; ResetAt is set only for a parseable bounce.
type Verdict struct {
	Status  Status
	ResetAt *time.Time
}

// ClassifyExit: the Claude CLI exits 0 on a usage limit, so the only trace is its short
// message, found in the stdout tail or the transcript's last assistant message.
func ClassifyExit(exitCode int, stdoutTail, lastAssistantMessage string, now time.Time) Verdict {
	for _, text := range []string{stdoutTail, lastAssistantMessage} {
		if isLimitMessage(text) {
			v := Verdict{Status: RateLimited}
			if t, ok := ParseRateLimitReset(text, now); ok {
				v.ResetAt = &t
			}
			return v
		}
	}
	if exitCode == 0 {
		return Verdict{Status: Completed}
	}
	return Verdict{Status: Failed}
}

// RateLimitPause is how long to sit out a bounce: until the reset time plus a buffer, or a
// fixed back-off when the message gave no time.
func RateLimitPause(resetAt *time.Time, now time.Time) time.Duration {
	if resetAt == nil {
		return RateLimitFallbackPause
	}
	d := resetAt.Sub(now)
	if d < 0 {
		d = 0
	}
	return d + rateLimitBuffer
}

// Dirs are where transcripts live; zero values mean the user's defaults.
type Dirs struct {
	Projects string // ~/.claude/projects
	AgyBrain string // ~/.gemini/antigravity-cli/brain
}

func (d Dirs) projects() string {
	if d.Projects != "" {
		return d.Projects
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func (d Dirs) brain() string {
	if d.AgyBrain != "" {
		return d.AgyBrain
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "antigravity-cli", "brain")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// FindTranscriptPath searches every project directory for the (effectively unique) session id,
// since the CLI files a transcript under a directory named after the cwd it ran in.
func FindTranscriptPath(sessionID, projectsDir string) string {
	if sessionID == "" {
		return ""
	}
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if c := filepath.Join(projectsDir, e.Name(), sessionID+".jsonl"); fileExists(c) {
			return c
		}
	}
	return ""
}

// ReadLastAssistantMessage is the text of the transcript's last assistant turn ("" when that
// turn is only a tool call, or the file is unreadable).
func ReadLastAssistantMessage(transcript string) string {
	b, err := os.ReadFile(transcript)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(lines[i]), &entry) != nil || entry.Type != "assistant" {
			continue
		}
		var s string
		if json.Unmarshal(entry.Message.Content, &s) == nil {
			return s
		}
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(entry.Message.Content, &blocks) != nil {
			return ""
		}
		var texts []string
		for _, bl := range blocks {
			if bl.Type == "text" {
				texts = append(texts, bl.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

func FindAgyTranscriptPath(conversationID, brainDir string) string {
	if conversationID == "" {
		return ""
	}
	c := filepath.Join(brainDir, conversationID, ".system_generated", "logs", "transcript.jsonl")
	if fileExists(c) {
		return c
	}
	return ""
}

const uuidPattern = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`

var (
	agyConvRE = regexp.MustCompile(`(?i)(?:Created conversation|Streaming conversation|Stream goroutine exited for|conversation(?:\s+id)?[:\s=]+)(` + uuidPattern + `)`)
	uuidRE    = regexp.MustCompile(`(?i)(` + uuidPattern + `)`)
	convWord  = regexp.MustCompile(`(?i)conversation`)
)

// ExtractAgyConversationID pulls the conversation id out of an agy log.
func ExtractAgyConversationID(log string) string {
	if log == "" {
		return ""
	}
	if m := agyConvRE.FindStringSubmatch(log); m != nil {
		return m[1]
	}
	for _, line := range regexp.MustCompile(`\r?\n`).Split(log, -1) {
		if convWord.MatchString(line) {
			if m := uuidRE.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

func ReadLastAgyAssistantMessage(transcript string) string {
	b, err := os.ReadFile(transcript)
	if err != nil {
		return ""
	}
	lines := regexp.MustCompile(`\r?\n`).Split(string(b), -1)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry struct {
			Source  string `json:"source"`
			Content any    `json:"content"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if s, ok := entry.Content.(string); ok && entry.Source == "MODEL" && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// FindLatestAgyTranscriptPath is the most recently modified agy transcript.
func FindLatestAgyTranscriptPath(brainDir string) string {
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return ""
	}
	var latest string
	var latestMod time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		c := filepath.Join(brainDir, e.Name(), ".system_generated", "logs", "transcript.jsonl")
		if st, err := os.Stat(c); err == nil && (latest == "" || st.ModTime().After(latestMod)) {
			latest, latestMod = c, st.ModTime()
		}
	}
	return latest
}

// ClassifySession classifies a finished run from its transcript. agent uses the ledger
// vocabulary ("claude"/"c", "agy"/"a").
func ClassifySession(agent, sessionID, logFile string, exitCode int, dirs Dirs, now time.Time) Verdict {
	var last string
	switch agent {
	case "claude", "c":
		if t := FindTranscriptPath(sessionID, dirs.projects()); t != "" {
			last = ReadLastAssistantMessage(t)
		}
	case "agy", "a":
		var transcript string
		if logFile != "" && fileExists(logFile) {
			if b, err := os.ReadFile(logFile); err == nil {
				if id := ExtractAgyConversationID(string(b)); id != "" {
					transcript = FindAgyTranscriptPath(id, dirs.brain())
				}
			}
		}
		if transcript == "" && sessionID != "" {
			transcript = FindAgyTranscriptPath(sessionID, dirs.brain())
		}
		if transcript != "" {
			last = ReadLastAgyAssistantMessage(transcript)
		}
	}
	return ClassifyExit(exitCode, "", last, now)
}
