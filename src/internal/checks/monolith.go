package checks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"work_dot_bat/src/internal/planstatus"
)

// DefaultMaxLines is the monolith threshold.
const DefaultMaxLines = 500

// Test suites reflect assertion count, not the "hard to read/modify" problem this check targets.
var excludeFileRE = regexp.MustCompile(`\.(test|spec)\.tsx?$`)

// Bundled/generated output, not hand-authored source.
const excludeDir = "generated"

var (
	tsFileRE  = regexp.MustCompile(`\.tsx?$`)
	cssFileRE = regexp.MustCompile(`\.css$`)
)

// CountLines counts lines the way the baseline does: one trailing newline is not a line.
func CountLines(content string) int {
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return 0
	}
	return strings.Count(trimmed, "\n") + 1
}

// MonolithBaseline is agent_docs/monolith_baseline.json: files grandfathered over the threshold at
// the line count they had, plus the reasons a human accepted some on purpose.
type MonolithBaseline struct {
	MaxLines *int              `json:"maxLines,omitempty"`
	Entries  map[string]int    `json:"entries"`
	Reasons  map[string]string `json:"reasons,omitempty"`
}

func (b MonolithBaseline) max() int {
	if b.MaxLines == nil {
		return DefaultMaxLines
	}
	return *b.MaxLines
}

func monolithFile(root string) string {
	return filepath.Join(root, "agent_docs", "monolith_baseline.json")
}

func ReadMonolithBaseline(root string) (MonolithBaseline, error) {
	n := DefaultMaxLines
	b, err := os.ReadFile(monolithFile(root))
	if errors.Is(err, os.ErrNotExist) {
		return MonolithBaseline{MaxLines: &n, Entries: map[string]int{}}, nil
	}
	if err != nil {
		return MonolithBaseline{}, err
	}
	var bl MonolithBaseline
	if err := json.Unmarshal(b, &bl); err != nil {
		return MonolithBaseline{}, err
	}
	if bl.Entries == nil {
		bl.Entries = map[string]int{}
	}
	return bl, nil
}

func writeMonolith(root string, b MonolithBaseline) error {
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(monolithFile(root), append(out, '\n'), 0o644)
}

// ListMonolithFiles is the hand-written source the check scans (sorted).
func ListMonolithFiles(root string) []string {
	return ListMonolithFilesWithConfig(root, ReadScanConfig(root))
}

func ListMonolithFilesWithConfig(root string, cfg ScanConfig) []string {
	var results []string
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			switch {
			case e.IsDir():
				if !cfg.IsExcludedDir(e.Name()) {
					walk(filepath.Join(dir, e.Name()))
				}
			case isMonolithFile(e.Name(), cfg):
				rel, _ := filepath.Rel(root, filepath.Join(dir, e.Name()))
				results = append(results, filepath.ToSlash(rel))
			}
		}
	}
	for _, d := range cfg.Dirs {
		walk(filepath.Join(root, filepath.FromSlash(d)))
	}
	sort.Strings(results)
	return results
}

func isMonolithFile(name string, cfg ScanConfig) bool {
	if len(cfg.Extensions) == 0 {
		return (tsFileRE.MatchString(name) && !excludeFileRE.MatchString(name)) || cssFileRE.MatchString(name)
	}
	return cfg.HasExtension(name)
}

type Scanned struct {
	Path  string
	Lines int
}

func ScanFiles(root string, files []string) ([]Scanned, error) {
	out := make([]Scanned, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return nil, err
		}
		out = append(out, Scanned{f, CountLines(string(b))})
	}
	return out, nil
}

type MonolithViolation struct {
	Path          string
	Lines         int
	BaselineLines int // grown violations only
}

// ClassifyMonolith is the pure comparison: over-threshold files not in the baseline are new; ones
// that grew past their baseline line count are grown.
func ClassifyMonolith(b MonolithBaseline, scanned []Scanned) (maxLines int, added, grown []MonolithViolation) {
	maxLines = b.max()
	for _, s := range scanned {
		if s.Lines <= maxLines {
			continue
		}
		base, ok := b.Entries[s.Path]
		switch {
		case !ok:
			added = append(added, MonolithViolation{Path: s.Path, Lines: s.Lines})
		case s.Lines > base:
			grown = append(grown, MonolithViolation{Path: s.Path, Lines: s.Lines, BaselineLines: base})
		}
	}
	return
}

func buildMonolith(max int, entries map[string]int, reasons map[string]string) MonolithBaseline {
	kept := map[string]string{}
	for p, r := range reasons {
		if _, ok := entries[p]; ok {
			kept[p] = r
		}
	}
	b := MonolithBaseline{MaxLines: &max, Entries: entries}
	if len(kept) > 0 {
		b.Reasons = kept
	}
	return b
}

// RegenerateMonolithBaseline rewrites the baseline from disk. By default the ratchet only shrinks:
// entries under the threshold are dropped and shrunken ones lowered. A new path or a raised value
// is refused (nothing written) unless named in accept, which needs a reason recorded in the
// baseline so the decision survives into review.
func RegenerateMonolithBaseline(root string, accept []string, reason string) (MonolithBaseline, []MonolithViolation, error) {
	if len(accept) > 0 && strings.TrimSpace(reason) == "" {
		return MonolithBaseline{}, nil, errors.New(`--accept needs a --reason "<text>" explaining why this file is allowed to stay over the threshold.`)
	}
	prev, err := ReadMonolithBaseline(root)
	if err != nil {
		return MonolithBaseline{}, nil, err
	}
	max := prev.max()
	scanned, err := ScanFiles(root, ListMonolithFiles(root))
	if err != nil {
		return MonolithBaseline{}, nil, err
	}
	over := map[string]int{}
	var overOrder []string
	for _, s := range scanned {
		if s.Lines > max {
			over[s.Path] = s.Lines
			overOrder = append(overOrder, s.Path)
		}
	}
	for _, p := range accept {
		if _, ok := over[p]; !ok {
			return MonolithBaseline{}, nil, fmt.Errorf("--accept %s: that file is not over the %d-line threshold, so there is nothing to accept.", p, max)
		}
	}
	entries := map[string]int{}
	reasons := map[string]string{}
	for k, v := range prev.Reasons {
		reasons[k] = v
	}
	var refused []MonolithViolation
	for _, p := range overOrder {
		lines := over[p]
		baseLines, inBaseline := prev.Entries[p]
		switch {
		case contains(accept, p):
			entries[p] = lines
			reasons[p] = strings.TrimSpace(reason)
		case !inBaseline || lines > baseLines:
			v := MonolithViolation{Path: p, Lines: lines, BaselineLines: -1}
			if inBaseline {
				v.BaselineLines = baseLines
			}
			refused = append(refused, v)
		default:
			entries[p] = lines
		}
	}
	bl := buildMonolith(max, entries, reasons)
	if len(refused) == 0 {
		if err := writeMonolith(root, bl); err != nil {
			return bl, nil, err
		}
	}
	return bl, refused, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func monthYearFolder(t time.Time) string {
	return strings.ToLower(t.UTC().Month().String()) + fmt.Sprintf("_%d", t.UTC().Year())
}

var (
	srcExtRE    = regexp.MustCompile(`\.(tsx?|css)$`)
	nonSlugRE   = regexp.MustCompile(`[^a-z0-9]+`)
	edgeUnderRE = regexp.MustCompile(`^_+|_+$`)
)

// BreakupPlanPath is where the stub plan for an oversized file lives.
func BreakupPlanPath(relSource string, now time.Time) string {
	ext := filepath.Ext(relSource)
	slug := strings.TrimSuffix(relSource, ext)
	slug = nonSlugRE.ReplaceAllString(strings.ToLower(slug), "_")
	slug = edgeUnderRE.ReplaceAllString(slug, "")
	return fmt.Sprintf("plans/%s/monolith_breakup/%s.md", monthYearFolder(now), slug)
}

// BreakupPlan is what ensuring a breakup plan did.
type BreakupPlan struct {
	Path            string
	Created, Reopen bool
}

// EnsureBreakupPlan mechanically writes a stub plan naming the offender (a git hook cannot design a
// sensible split) and registers it. A plan that exists is left alone — unless it is Done/Cancelled,
// which still describes the file as it was when the split landed: then it is reopened with a dated
// `## Regrowth` section, unless a human accepted the file (a `reasons` entry).
func EnsureBreakupPlan(root string, v MonolithViolation, accepted bool, now time.Time) (BreakupPlan, error) {
	entryPath := BreakupPlanPath(v.Path, now)
	full := filepath.Join(root, "agent_docs", filepath.FromSlash(entryPath))
	today := now.UTC().Format("2006-01-02")

	if _, err := os.Stat(full); err == nil {
		status := ""
		for _, e := range planstatus.ReadStatus(root) {
			if e.Path == entryPath {
				status = e.Status
			}
		}
		reopened := !accepted && (status == "Done" || status == "Cancelled")
		if reopened {
			if err := planstatus.Set(root, entryPath, "Not Started", false); err != nil {
				return BreakupPlan{}, err
			}
			f, err := os.OpenFile(full, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				return BreakupPlan{}, err
			}
			fmt.Fprintf(f, "\n## Regrowth — %s: %d lines\n\n`%s` grew back over the monolith threshold after this plan was marked %s, "+
				"so the resolution above no longer describes the file. Re-run the split (or `--accept` it with a written "+
				"reason: `work_dot_bat check-monolith baseline --accept %s --reason \"<text>\"`).\n", today, v.Lines, v.Path, status, v.Path)
			f.Close()
		}
		return BreakupPlan{Path: entryPath, Reopen: reopened}, nil
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return BreakupPlan{}, err
	}
	content := "# GOAL\n" +
		fmt.Sprintf("`%s` is %d lines, over the monolith threshold enforced by `work_dot_bat check-monolith check`.\n\n", v.Path, v.Lines) +
		"# Context\n" +
		fmt.Sprintf("- Flagged automatically by the monolith check on %s.\n", today) +
		"- See `agent_docs/instructions/high_trust/monolith-breakup.md` for how to analyze and break up this file.\n\n" +
		"# Research\n\n# Implementation Plan\n\n# Testing Plan\n"
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return BreakupPlan{}, err
	}
	registered := false
	for _, e := range planstatus.ReadStatus(root) {
		if e.Path == entryPath {
			registered = true
		}
	}
	if !registered {
		if err := planstatus.Add(root, entryPath, "Not Started"); err != nil {
			return BreakupPlan{}, err
		}
	}
	return BreakupPlan{Path: entryPath, Created: true}, nil
}

// MonolithResult is the outcome of the check.
type MonolithResult struct {
	MaxLines  int
	New, Grew []MonolithViolation
	Plans     []BreakupPlan
	Tightened bool
}

// RunMonolithCheck classifies the tree, tightens the baseline when everything passes (unless
// noTighten), and writes/reopens a breakup plan for each violation.
func RunMonolithCheck(root string, noTighten bool, now time.Time) (MonolithResult, error) {
	baseline, err := ReadMonolithBaseline(root)
	if err != nil {
		return MonolithResult{}, err
	}
	scanned, err := ScanFiles(root, ListMonolithFiles(root))
	if err != nil {
		return MonolithResult{}, err
	}
	max, added, grown := ClassifyMonolith(baseline, scanned)
	violations := append(append([]MonolithViolation{}, added...), grown...)

	tightened := false
	updated := map[string]int{}
	paths := make([]string, 0, len(baseline.Entries))
	for p := range baseline.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	byPath := map[string]int{}
	for _, s := range scanned {
		byPath[s.Path] = s.Lines
	}
	for _, p := range paths {
		baseLines := baseline.Entries[p]
		lines, ok := byPath[p]
		switch {
		case !ok || lines <= max:
			tightened = true
		case lines < baseLines:
			updated[p] = lines
			tightened = true
		default:
			updated[p] = baseLines
		}
	}
	if len(violations) == 0 && tightened && !noTighten {
		if err := writeMonolith(root, buildMonolith(max, updated, baseline.Reasons)); err != nil {
			return MonolithResult{}, err
		}
	}
	res := MonolithResult{MaxLines: max, New: added, Grew: grown, Tightened: tightened}
	for _, v := range violations {
		plan, err := EnsureBreakupPlan(root, v, baseline.Reasons[v.Path] != "", now)
		if err != nil {
			return res, err
		}
		res.Plans = append(res.Plans, plan)
	}
	return res, nil
}

func flagValues(args []string, flag string) ([]string, error) {
	var values []string
	for i, a := range args {
		if a != flag {
			continue
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return nil, fmt.Errorf("%s needs a value.", flag)
		}
		values = append(values, args[i+1])
	}
	return values, nil
}

func hasFlag(args []string, flag string) bool { return contains(args, flag) }

// MonolithMain is `check-monolith <check|baseline [--accept <path> --reason <text>]>`.
func MonolithMain(root string, argv []string, out, errOut io.Writer, now time.Time) int {
	cmd := ""
	var rest []string
	if len(argv) > 0 {
		cmd, rest = argv[0], argv[1:]
	}
	fail := func(err error) int { fmt.Fprintln(errOut, err.Error()); return 1 }
	switch cmd {
	case "check":
		noTighten := hasFlag(rest, "--no-tighten")
		res, err := RunMonolithCheck(root, noTighten, now)
		if err != nil {
			return fail(err)
		}
		violations := len(res.New) + len(res.Grew)
		if violations == 0 {
			if res.Tightened && !noTighten {
				fmt.Fprintln(out, "Monolith check passed and baseline tightened.")
			} else {
				fmt.Fprintf(out, "Monolith check passed (%d-line threshold).\n", res.MaxLines)
			}
			return 0
		}
		fmt.Fprintf(out, "Monolith check found %d file(s) exceeding %d lines:\n", violations, res.MaxLines)
		for _, v := range res.New {
			fmt.Fprintf(out, "  + NEW  %s (%d lines)\n", v.Path, v.Lines)
		}
		for _, v := range res.Grew {
			fmt.Fprintf(out, "  ^ GREW %s (%d lines, baseline was %d)\n", v.Path, v.Lines, v.BaselineLines)
		}
		for _, p := range res.Plans {
			verb := "already tracked"
			if p.Created {
				verb = "wrote"
			} else if p.Reopen {
				verb = "reopened (file regrew)"
			}
			fmt.Fprintf(out, "    -> %s plan: agent_docs/%s\n", verb, p.Path)
		}
		fmt.Fprintln(out, "\nEach file needs a shrink, or a deliberate and justified baseline entry: "+
			"`work_dot_bat check-monolith baseline --accept <path> --reason \"<why>\"`.")
		return 1
	case "baseline":
		accept, err := flagValues(rest, "--accept")
		if err != nil {
			return fail(err)
		}
		reasons, err := flagValues(rest, "--reason")
		if err != nil {
			return fail(err)
		}
		reason := ""
		if len(reasons) > 0 {
			reason = reasons[0]
		}
		bl, refused, err := RegenerateMonolithBaseline(root, accept, reason)
		if err != nil {
			return fail(err)
		}
		if len(refused) > 0 {
			fmt.Fprintf(out, "Baseline NOT updated — it only shrinks by default. Refused %d file(s) over %d lines:\n", len(refused), bl.max())
			for _, r := range refused {
				was := "not in the baseline"
				if r.BaselineLines >= 0 {
					was = fmt.Sprintf("baseline was %d", r.BaselineLines)
				}
				fmt.Fprintf(out, "  %s (%d lines, %s)\n", r.Path, r.Lines, was)
			}
			fmt.Fprintln(out, "\nSplit the file, or accept it deliberately: "+
				"`work_dot_bat check-monolith baseline --accept <path> --reason \"<why>\"` "+
				"(the reason is recorded in agent_docs/monolith_baseline.json).")
			return 1
		}
		fmt.Fprintf(out, "Baseline updated: %d file(s) recorded over %d lines.\n", len(bl.Entries), bl.max())
		return 0
	}
	fmt.Fprintf(errOut, "Unknown command: \"%s\"\nUsage: check-monolith <check|baseline [--accept <path> --reason <text>]>\n", cmd)
	return 1
}
