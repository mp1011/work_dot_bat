// Package syncer keeps the copies of vendored files inside a target repo current.
//
// Per file, the target's copy is classified against the tool's manifest:
//
//	missing    not installed yet                                  -> install
//	current    same version, body untouched                       -> nothing
//	outdated   older version, body untouched                      -> update
//	modified   body hash differs from its own header (local edit) -> ask; never silent
//	unmanaged  exists but has no managed header                   -> ask; never silent
//	newer      target version is ahead of the tool's              -> warn, never downgrade
//
// Files that carry a managed header but are no longer in the manifest are reported ("orphans"),
// not deleted. Sync never commits.
package syncer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"work_dot_bat/internal/managed"
)

// TargetDir is where vendored files live inside a target repo.
const TargetDir = "scripts/work_dot_bat"

type State string

const (
	Missing   State = "missing"
	Current   State = "current"
	Outdated  State = "outdated"
	Modified  State = "modified"
	Unmanaged State = "unmanaged"
	Newer     State = "newer"
)

type Item struct {
	File      string
	Version   string // the tool's
	State     State
	Installed string // the target's, when it has a managed header
}

type Plan struct {
	Items   []Item
	Orphans []string
}

func targetPath(root, file string) string { return filepath.Join(root, filepath.FromSlash(file)) }

// Classify compares the target's text (nil = absent) with the manifest entry.
func Classify(entry managed.Entry, target *string) (State, string) {
	if target == nil {
		return Missing, ""
	}
	p := managed.Parse(*target)
	if p == nil {
		return Unmanaged, ""
	}
	if managed.HashBody(p.Body) != p.SHA256 {
		return Modified, p.Version
	}
	cmp, err := managed.CompareVersions(p.Version, entry.Version)
	if err != nil {
		return Outdated, p.Version
	}
	switch {
	case cmp == 0:
		// Same version but a different body than the release: outdated, so it is repaired.
		if p.SHA256 == entry.SHA256 {
			return Current, p.Version
		}
		return Outdated, p.Version
	case cmp > 0:
		return Newer, p.Version
	}
	return Outdated, p.Version
}

func readOpt(p string) *string {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// PlanSync classifies every manifest file in root and lists orphaned managed files.
func PlanSync(root string, m managed.Manifest) Plan {
	var plan Plan
	files := make([]string, 0, len(m.Files))
	for f := range m.Files {
		files = append(files, f)
	}
	sort.Strings(files)
	known := map[string]bool{}
	for _, file := range files {
		entry := m.Files[file]
		full := targetPath(root, file)
		known[full] = true
		state, installed := Classify(entry, readOpt(full))
		plan.Items = append(plan.Items, Item{File: file, Version: entry.Version, State: state, Installed: installed})
	}
	base := targetPath(root, TargetDir)
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || known[p] {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && managed.Parse(string(b)) != nil {
			rel, _ := filepath.Rel(root, p)
			plan.Orphans = append(plan.Orphans, filepath.ToSlash(rel))
		}
		return nil
	})
	return plan
}

// StaleItems is everything sync would change or that deserves attention (not just current).
func (p Plan) StaleItems() []Item {
	var out []Item
	for _, i := range p.Items {
		if i.State != Current && i.State != Newer {
			out = append(out, i)
		}
	}
	return out
}

type Decision int

const (
	Skip Decision = iota
	Overwrite
)

type Result struct{ Written, Skipped, Newer []Item }

// Options for Apply.
type Options struct {
	// Decide is asked for modified/unmanaged files only; nil skips them.
	Decide func(Item) Decision
	DryRun bool
	// Today stamps the date of entries that carry none.
	Today string
}

// Apply installs or updates the files, returning what was written, skipped and left newer.
func Apply(root string, plan Plan, m managed.Manifest, vendor fs.FS, o Options) (Result, error) {
	var res Result
	for _, item := range plan.Items {
		switch item.State {
		case Current:
			continue
		case Newer:
			res.Newer = append(res.Newer, item)
			continue
		}
		if (item.State == Modified || item.State == Unmanaged) && (o.Decide == nil || o.Decide(item) != Overwrite) {
			res.Skipped = append(res.Skipped, item)
			continue
		}
		if !o.DryRun {
			entry := m.Files[item.File]
			body, err := managed.ReadBody(vendor, item.File)
			if err != nil {
				return res, err
			}
			date := entry.Date
			if date == "" {
				date = o.Today
			}
			text, err := managed.Render(item.File, entry.Version, date, body)
			if err != nil {
				return res, err
			}
			full := targetPath(root, item.File)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return res, err
			}
			if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
				return res, err
			}
			if strings.HasSuffix(item.File, ".sh") {
				_ = os.Chmod(full, 0o755) // best effort; not meaningful on every filesystem
			}
		}
		res.Written = append(res.Written, item)
	}
	return res, nil
}

// FormatReport is the text `sync` prints.
func FormatReport(plan Plan, res Result, dryRun bool) string {
	var lines []string
	verb := "Wrote"
	if dryRun {
		verb = "Would write"
	}
	for _, i := range res.Written {
		detail := "new"
		if i.State != Missing {
			detail = i.Installed + " -> " + i.Version
		}
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", verb, i.File, detail))
	}
	for _, i := range res.Skipped {
		why := "exists without a managed header"
		if i.State == Modified {
			why = "edited locally"
		}
		lines = append(lines, fmt.Sprintf("Skipped: %s (%s) — re-run and confirm to overwrite", i.File, why))
	}
	for _, i := range res.Newer {
		lines = append(lines, fmt.Sprintf("Warning: %s is version %s, newer than this tool's %s; left alone (update work_dot_bat).", i.File, i.Installed, i.Version))
	}
	for _, o := range plan.Orphans {
		lines = append(lines, fmt.Sprintf("Note: %s is a managed file this tool no longer ships; delete it if unused.", o))
	}
	if len(lines) == 0 {
		lines = append(lines, "Everything is up to date.")
	}
	if len(res.Written) > 0 && !dryRun {
		lines = append(lines, "Changes are in the working tree only; review and commit them yourself.")
	}
	return strings.Join(lines, "\n")
}

// IsMainCheckout: root has a .git directory (a linked worktree has a .git file). Sync only runs
// against the main checkout; worktrees receive changes through ordinary git merges.
func IsMainCheckout(root string) bool {
	st, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil && st.IsDir()
}
