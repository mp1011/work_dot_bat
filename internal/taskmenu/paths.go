package taskmenu

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, `"`), `'`)
	return strings.TrimSuffix(strings.TrimSuffix(s, `"`), `'`)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func hasParentSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ResolvePlanPath turns whatever the user typed or pasted (relative, absolute, with or
// without agent_docs/ or plans/) into an agent_docs-relative path. "" means the input is not
// a path inside agent_docs.
func ResolvePlanPath(raw, repoRoot string) string {
	cleaned := toSlash(stripQuotes(raw))
	if cleaned == "" {
		return ""
	}
	docsDir := toSlash(filepath.Join(repoRoot, "agent_docs"))
	root := toSlash(repoRoot)
	var entry string
	switch {
	case hasPrefixFold(cleaned, "agent_docs/"):
		entry = cleaned[len("agent_docs/"):]
	case filepath.IsAbs(cleaned):
		switch {
		case hasPrefixFold(cleaned, docsDir+"/"):
			entry = cleaned[len(docsDir)+1:]
		case hasPrefixFold(cleaned, root+"/agent_docs/"):
			entry = cleaned[len(root+"/agent_docs/"):]
		default:
			return ""
		}
	default:
		docs := filepath.Join(repoRoot, "agent_docs")
		if fileExists(filepath.Join(docs, filepath.FromSlash(cleaned))) || strings.HasPrefix(cleaned, "plans/") {
			entry = cleaned
		} else if cand := path.Join("plans", cleaned); fileExists(filepath.Join(docs, filepath.FromSlash(cand))) {
			entry = cand
		} else {
			entry = cleaned
		}
	}
	entry = strings.TrimLeft(toSlash(entry), "/")
	if hasPrefixFold(entry, "agent_docs/") {
		entry = entry[len("agent_docs/"):]
	}
	if hasParentSegment(entry) {
		return ""
	}
	return entry
}

// PlanFolders lists "plans" and every folder beneath it except the done/old_plans archives.
func PlanFolders(repoRoot string) []string {
	docs := filepath.Join(repoRoot, "agent_docs")
	folders := []string{"plans"}
	var scan func(dir string)
	scan = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "old_plans" || e.Name() == "done" {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if rel, err := filepath.Rel(docs, full); err == nil {
				folders = append(folders, filepath.ToSlash(rel))
			}
			scan(full)
		}
	}
	scan(filepath.Join(docs, "plans"))
	sort.Strings(folders)
	return folders
}
