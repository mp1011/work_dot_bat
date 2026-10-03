// Package pathrewrite is the one-shot rewrite of references to the old script locations
// (scripts/<x>.sh, scripts/lib/<x>.mjs) into the managed folder (scripts/work_dot_bat/...), used
// when a repo that already had these scripts moves onto work_dot_bat. Docs, skills, hooks,
// workflows, package.json, prompts and tests are all just text, so one path map handles them.
package pathrewrite

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"work_dot_bat/internal/managed"
	"work_dot_bat/internal/syncer"
)

// Plan files are history; the managed folder holds copies that must stay byte-exact.
var excludedPrefixes = []string{"agent_docs/plans/", syncer.TargetDir + "/", "node_modules/", ".git/"}

var textExtensions = map[string]bool{
	".md": true, ".mjs": true, ".js": true, ".cjs": true, ".ts": true, ".tsx": true, ".json": true,
	".yml": true, ".yaml": true, ".sh": true, ".html": true, ".txt": true, ".bat": true,
}

type Mapping struct{ From, To string }

// BuildPathMap maps each shipped script's old location to its managed one. Manifest keys look
// like `scripts/work_dot_bat/lib/plan-status.mjs`; the old location drops the `work_dot_bat/`
// segment. Longest first, so a longer path is never shadowed by a prefix of it.
func BuildPathMap(m managed.Manifest) []Mapping {
	var out []Mapping
	prefix := syncer.TargetDir + "/"
	for file := range m.Files {
		if !strings.HasPrefix(file, prefix) {
			continue
		}
		rest := file[len(prefix):]
		from := "scripts/" + rest
		if strings.HasPrefix(rest, "typescript/") {
			// The TypeScript-only Node checks used to live in lib/ next to everything else.
			from = "scripts/lib/" + strings.TrimPrefix(rest, "typescript/")
		} else if !strings.HasPrefix(rest, "lib/") && strings.Contains(rest, "/") {
			continue
		}
		out = append(out, Mapping{From: from, To: file})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].From) != len(out[j].From) {
			return len(out[i].From) > len(out[j].From)
		}
		return out[i].From < out[j].From
	})
	return out
}

func isWord(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// RewriteText replaces each old path that stands alone: not preceded by a word character or
// dash, and not followed by one (or by a dot and a word character, e.g. `x.sh.bak`). Already
// rewritten text is left alone, since the new path never matches the old. Returns the new text
// and how many references changed.
func RewriteText(text string, m []Mapping) (string, int) {
	if len(m) == 0 {
		return text, 0
	}
	var out strings.Builder
	count := 0
	for i := 0; i < len(text); {
		replaced := false
		if i == 0 || !(isWord(text[i-1]) || text[i-1] == '-') {
			for _, mp := range m {
				if !strings.HasPrefix(text[i:], mp.From) {
					continue
				}
				end := i + len(mp.From)
				if end < len(text) {
					c := text[end]
					if isWord(c) || c == '-' || (c == '.' && end+1 < len(text) && isWord(text[end+1])) {
						continue
					}
				}
				out.WriteString(mp.To)
				i, count, replaced = end, count+1, true
				break
			}
		}
		if !replaced {
			out.WriteByte(text[i])
			i++
		}
	}
	return out.String(), count
}

func IsRewritable(file string) bool {
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(file, p) {
			return false
		}
	}
	ext := strings.ToLower(filepath.Ext(file))
	if ext == "" {
		return strings.HasPrefix(file, ".husky/") // hooks have no extension
	}
	return textExtensions[ext]
}

func ListTrackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

type Change struct {
	File  string
	Count int
}

// RewriteRepo lists every file that changes (and writes them only when apply is true).
func RewriteRepo(root string, m []Mapping, apply bool, files []string) ([]Change, error) {
	if files == nil {
		var err error
		if files, err = ListTrackedFiles(root); err != nil {
			return nil, err
		}
	}
	var changed []Change
	for _, file := range files {
		if !IsRewritable(file) {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(file))
		raw, err := os.ReadFile(full)
		if err != nil || bytes.IndexByte(raw, 0) >= 0 {
			continue
		}
		text, n := RewriteText(string(raw), m)
		if n == 0 {
			continue
		}
		changed = append(changed, Change{file, n})
		if apply {
			if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}
