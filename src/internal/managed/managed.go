// Package managed is the "managed file" header that marks a file as copied into a target repo
// by work_dot_bat, and the vendor manifest that versions those files.
//
//	// work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.
//	// file: scripts/work_dot_bat/lib/plan-status.mjs
//	// version: 1.4.0
//	// date: 2026-10-03
//	// sha256: <hash of the body>
//
// The body is the file's canonical content without the header (as stored in vendor/). Its hash
// is what lets sync tell an untouched copy from a locally edited one.
package managed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const Marker = "work_dot_bat managed file"
const headerLines = 5

// Style is a file type's comment syntax.
type Style struct{ Open, Close string }

var (
	shRE   = regexp.MustCompile(`\.sh$`)
	jsRE   = regexp.MustCompile(`\.(mjs|js|cjs|ts|mts|tsx)$`)
	mdRE   = regexp.MustCompile(`\.(md|html)$`)
	eolRE  = regexp.MustCompile(`\r\n`)
	verRE  = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// CommentStyle returns the comment syntax for file, or nil when it has no safe one (JSON).
func CommentStyle(file string) *Style {
	switch {
	case shRE.MatchString(file):
		return &Style{"# ", ""}
	case jsRE.MatchString(file):
		return &Style{"// ", ""}
	case mdRE.MatchString(file):
		return &Style{"<!-- ", " -->"}
	}
	return nil
}

func NormalizeEOL(text string) string { return eolRE.ReplaceAllString(text, "\n") }

func HashBody(body string) string {
	sum := sha256.Sum256([]byte(NormalizeEOL(body)))
	return hex.EncodeToString(sum[:])
}

// ParseVersion reads a plain major.minor.patch.
func ParseVersion(v string) ([3]int, error) {
	m := verRE.FindStringSubmatch(v)
	if m == nil {
		return [3]int{}, fmt.Errorf("Invalid version %q (expected major.minor.patch)", v)
	}
	var out [3]int
	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out, nil
}

// CompareVersions is -1, 0 or 1.
func CompareVersions(a, b string) (int, error) {
	pa, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func BumpVersion(v, level string) (string, error) {
	p, err := ParseVersion(v)
	if err != nil {
		return "", err
	}
	switch level {
	case "major":
		return fmt.Sprintf("%d.0.0", p[0]+1), nil
	case "minor":
		return fmt.Sprintf("%d.%d.0", p[0], p[1]+1), nil
	case "patch", "":
		return fmt.Sprintf("%d.%d.%d", p[0], p[1], p[2]+1), nil
	}
	return "", fmt.Errorf("Unknown bump level %q", level)
}

// Render is body with the managed header; a leading shebang stays the first line.
func Render(file, version, date, body string) (string, error) {
	style := CommentStyle(file)
	if style == nil {
		return "", fmt.Errorf("Cannot add a managed-file header to %s: no comment syntax for this file type", file)
	}
	text := NormalizeEOL(body)
	lines := []string{
		Marker + " — DO NOT EDIT; overwritten by `work_dot_bat sync`.",
		"file: " + file,
		"version: " + version,
		"date: " + date,
		"sha256: " + HashBody(text),
	}
	for i, l := range lines {
		lines[i] = style.Open + l + style.Close
	}
	header := strings.Join(lines, "\n") + "\n"
	if strings.HasPrefix(text, "#!") {
		nl := strings.Index(text, "\n")
		return text[:nl+1] + header + text[nl+1:], nil
	}
	return header + text, nil
}

// Info is a parsed managed header plus the body it wraps.
type Info struct {
	File, Version, Date, SHA256, Body string
}

// Parse reads a managed header, or returns nil when the text carries none.
func Parse(text string) *Info {
	lines := strings.Split(NormalizeEOL(text), "\n")
	start := 0
	if strings.HasPrefix(lines[0], "#!") {
		start = 1
	}
	if start >= len(lines) || !strings.Contains(lines[start], Marker) {
		return nil
	}
	field := func(i int, key string) string {
		idx := start + i
		if idx >= len(lines) {
			return ""
		}
		m := regexp.MustCompile(key + `:\s*(.*?)\s*(?:-->)?\s*$`).FindStringSubmatch(lines[idx])
		if m == nil {
			return ""
		}
		return m[1]
	}
	info := &Info{File: field(1, "file"), Version: field(2, "version"), Date: field(3, "date"), SHA256: field(4, "sha256")}
	if info.File == "" || info.Version == "" || info.Date == "" || info.SHA256 == "" {
		return nil
	}
	rest := lines[min(len(lines), start+headerLines):]
	info.Body = strings.Join(append(append([]string{}, lines[:start]...), rest...), "\n")
	return info
}

// ValidDate reports whether s is YYYY-MM-DD.
func ValidDate(s string) bool { return dateRE.MatchString(s) }
