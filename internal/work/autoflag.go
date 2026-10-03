package work

import (
	"errors"
	"regexp"
	"strings"
)

// The human-only "Set auto: true" action. `auto: true` is the human's switch: this is the one
// writer for it, reachable only from this interactive launcher (a PreToolUse hook stops an
// agent from invoking the launcher), and deliberately not part of planstatus.

var (
	autoFrontmatterRE = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---`)
	autoTrueRE        = regexp.MustCompile(`(?m)^auto:\s*true\s*$`)
	statusLineRE      = regexp.MustCompile(`^status:\s*`)
)

// InsertAutoFlag adds `auto: true` after the status line (or at the end of the block), keeping
// the file's line endings. A file already carrying it is returned unchanged.
func InsertAutoFlag(content string) (string, error) {
	loc := autoFrontmatterRE.FindStringSubmatchIndex(content)
	if loc == nil {
		return "", errors.New("Plan file has no frontmatter block")
	}
	block := content[loc[2]:loc[3]]
	if autoTrueRE.MatchString(block) {
		return content, nil
	}
	eol := "\n"
	if strings.Contains(content[loc[0]:loc[1]], "\r\n") {
		eol = "\r\n"
	}
	lines := regexp.MustCompile(`\r?\n`).Split(block, -1)
	at := len(lines)
	for i, l := range lines {
		if statusLineRE.MatchString(l) {
			at = i + 1
			break
		}
	}
	lines = append(lines[:at], append([]string{"auto: true"}, lines[at:]...)...)
	return content[:loc[0]] + "---" + eol + strings.Join(lines, eol) + eol + "---" + content[loc[1]:], nil
}

// Runner runs a command and returns its exit code.
type Runner func(cmd string, args ...string) int

// CommitAutoFlag stages only the one plan file (never -A, so unrelated dirty state is not swept
// in) and commits it. Outcome is "committed", "add-failed" or "commit-failed".
func CommitAutoFlag(planPath string, run Runner) string {
	if run("git", "add", planPath) != 0 {
		return "add-failed"
	}
	if run("git", "commit", "-m", "Mark "+planPath+" auto: true") != 0 {
		return "commit-failed"
	}
	return "committed"
}
