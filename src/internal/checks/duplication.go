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
)

// Duplicated-block gate, a ratchet like the monolith one. The number is meaningless without the
// method, so do not change it without re-seeding agent_docs/duplication_baseline.json:
//   - Input set: `git ls-tree -r --name-only HEAD -- src`, filtered to .ts/.tsx, excluding files
//     with `.test.` in the name and any path containing `generated`. A directory pathspec is used
//     (not a glob) so top-level files like src/App.tsx are not silently dropped.
//   - Normalize: trim each line; drop blank lines and lines starting with `//`, `*`, or `/*`.
//   - Window: 8 consecutive normalized lines, joined with a newline.
//   - Count: distinct windows occurring in 2 or more distinct files.

const (
	WindowSize    = 8
	TopPairsLimit = 10
)

var (
	trackedTSRE   = regexp.MustCompile(`\.tsx?$`)
	excludeTestRE = regexp.MustCompile(`\.test\.`)
	excludeGenRE  = regexp.MustCompile(`generated`)
)

func IsTrackedSourceFile(rel string) bool {
	return IsTrackedSourceFileWithConfig(rel, DefaultScanConfig())
}

func IsTrackedSourceFileWithConfig(rel string, cfg ScanConfig) bool {
	if cfg.IsPathExcluded(rel) {
		return false
	}
	if len(cfg.Extensions) == 0 {
		return trackedTSRE.MatchString(rel) && !excludeTestRE.MatchString(rel) && !excludeGenRE.MatchString(rel)
	}
	return cfg.HasExtension(rel)
}

// ListTrackedSourceFiles is the git-tracked source set (sorted); it can list a path the working
// tree no longer has, which callers skip.
func ListTrackedSourceFiles(root string, git GitFunc) ([]string, error) {
	return ListTrackedSourceFilesWithConfig(root, ReadScanConfig(root), git)
}

func ListTrackedSourceFilesWithConfig(root string, cfg ScanConfig, git GitFunc) ([]string, error) {
	args := append([]string{"ls-tree", "-r", "--name-only", "HEAD", "--"}, cfg.Dirs...)
	out, err := git(root, args...)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range lineSplitRE.Split(out, -1) {
		if l = strings.TrimSpace(l); l != "" && IsTrackedSourceFileWithConfig(l, cfg) {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	return files, nil
}

// NormalizeLines trims lines and drops blanks and comment lines.
func NormalizeLines(content string) []string {
	var out []string
	for _, l := range lineSplitRE.Split(content, -1) {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "/*") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// ExtractWindows are every run of windowSize consecutive elements joined with delimiter.
func ExtractWindows(elements []string, windowSize int, delimiter string) []string {
	var w []string
	for i := 0; i+windowSize <= len(elements); i++ {
		w = append(w, strings.Join(elements[i:i+windowSize], delimiter))
	}
	return w
}

// FileWindows is one file's distinct windows.
type FileWindows struct {
	Path    string
	Windows []string // distinct, in first-seen order
}

// Duplicate is a window found in two or more files.
type Duplicate struct {
	Window string
	Files  []string // sorted
}

// FindDuplicates is the pure comparison, in the order windows were first seen (so ties in the
// reported pairs are deterministic).
func FindDuplicates(fw []FileWindows) []Duplicate {
	files := map[string][]string{}
	var order []string
	for _, f := range fw {
		for _, w := range f.Windows {
			if _, ok := files[w]; !ok {
				order = append(order, w)
			}
			if l := files[w]; len(l) == 0 || l[len(l)-1] != f.Path {
				files[w] = append(files[w], f.Path)
			}
		}
	}
	var dups []Duplicate
	for _, w := range order {
		if fs := files[w]; len(fs) >= 2 {
			s := append([]string{}, fs...)
			sort.Strings(s)
			dups = append(dups, Duplicate{w, s})
		}
	}
	return dups
}

type PairCount struct {
	Pair  string
	Count int
}

// TopFilePairs are the file pairs sharing the most duplicated blocks.
func TopFilePairs(dups []Duplicate, limit int) []PairCount {
	counts := map[string]int{}
	var order []string
	for _, d := range dups {
		for i := 0; i < len(d.Files); i++ {
			for j := i + 1; j < len(d.Files); j++ {
				k := d.Files[i] + " <-> " + d.Files[j]
				if _, ok := counts[k]; !ok {
					order = append(order, k)
				}
				counts[k]++
			}
		}
	}
	pairs := make([]PairCount, len(order))
	for i, k := range order {
		pairs[i] = PairCount{k, counts[k]}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].Count > pairs[j].Count })
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	return pairs
}

// DuplicationBaseline is agent_docs/duplication_baseline.json (also the structural variant's shape).
type DuplicationBaseline struct {
	TotalDuplicatedBlocks int    `json:"totalDuplicatedBlocks"`
	Note                  string `json:"note,omitempty"`
}

func readDupBaseline(file string) (DuplicationBaseline, error) {
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return DuplicationBaseline{}, nil
	}
	if err != nil {
		return DuplicationBaseline{}, err
	}
	var bl DuplicationBaseline
	return bl, json.Unmarshal(b, &bl)
}

func writeDupBaseline(file string, b DuplicationBaseline) error {
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(out, '\n'), 0o644)
}

func dupBaselineFile(root string) string {
	return filepath.Join(root, "agent_docs", "duplication_baseline.json")
}

type DupCount struct {
	FileCount, Total int
	TopPairs         []PairCount
}

// CountDuplication counts duplicated 8-line blocks across the tracked source.
func CountDuplication(root string, git GitFunc) (DupCount, error) {
	files, err := ListTrackedSourceFiles(root, git)
	if err != nil {
		return DupCount{}, err
	}
	var fw []FileWindows
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue // listed by HEAD but gone from the working tree: reflect the working tree
		}
		seen := map[string]bool{}
		var distinct []string
		for _, w := range ExtractWindows(NormalizeLines(string(b)), WindowSize, "\n") {
			if !seen[w] {
				seen[w] = true
				distinct = append(distinct, w)
			}
		}
		fw = append(fw, FileWindows{rel, distinct})
	}
	dups := FindDuplicates(fw)
	return DupCount{len(files), len(dups), TopFilePairs(dups, TopPairsLimit)}, nil
}

// DupCheck is the outcome of the duplication check.
type DupCheck struct {
	DupCount
	BaselineCount int
	Passed        bool
	Tightened     bool
}

const dupNote = "See scripts/work_dot_bat/lib/check-duplication.mjs for the exact method. Lower is better; this file is a ratchet."

// UpdateDuplicationBaseline rewrites the baseline to the current count; it refuses to raise it
// unless allowed, so the ratchet only tightens by default.
func UpdateDuplicationBaseline(root string, allowRegression bool, git GitFunc) (DuplicationBaseline, error) {
	prev, err := readDupBaseline(dupBaselineFile(root))
	if err != nil {
		return prev, err
	}
	res, err := CountDuplication(root, git)
	if err != nil {
		return prev, err
	}
	if !allowRegression && res.Total > prev.TotalDuplicatedBlocks {
		return prev, fmt.Errorf("Refusing to raise the duplication baseline from %d to %d without --allow-regression.", prev.TotalDuplicatedBlocks, res.Total)
	}
	next := DuplicationBaseline{res.Total, dupNote}
	return next, writeDupBaseline(dupBaselineFile(root), next)
}

func CheckDuplication(root string, noTighten bool, git GitFunc) (DupCheck, error) {
	prev, err := readDupBaseline(dupBaselineFile(root))
	if err != nil {
		return DupCheck{}, err
	}
	res, err := CountDuplication(root, git)
	if err != nil {
		return DupCheck{}, err
	}
	c := DupCheck{DupCount: res, BaselineCount: prev.TotalDuplicatedBlocks, Passed: res.Total <= prev.TotalDuplicatedBlocks}
	if c.Passed && res.Total < prev.TotalDuplicatedBlocks {
		if !noTighten {
			if _, err := UpdateDuplicationBaseline(root, false, git); err != nil {
				return c, err
			}
		}
		c.Tightened = true
	}
	return c, nil
}

// DuplicationMain is `check-duplication <check|update> [--allow-regression] [--no-tighten]`.
func DuplicationMain(root string, argv []string, out, errOut io.Writer, git GitFunc) int {
	cmd := ""
	var rest []string
	if len(argv) > 0 {
		cmd, rest = argv[0], argv[1:]
	}
	switch cmd {
	case "check":
		noTighten := hasFlag(rest, "--no-tighten")
		r, err := CheckDuplication(root, noTighten, git)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 1
		}
		if r.Passed {
			if r.Tightened && !noTighten {
				fmt.Fprintf(out, "Duplication check passed and baseline tightened (%d -> %d).\n", r.BaselineCount, r.Total)
			} else {
				fmt.Fprintf(out, "Duplication check passed (%d <= baseline %d).\n", r.Total, r.BaselineCount)
			}
			return 0
		}
		fmt.Fprintf(out, "Duplication check failed: %d duplicated 8-line blocks (baseline %d).\n", r.Total, r.BaselineCount)
		fmt.Fprintln(out, "Top offending file pairs:")
		for _, p := range r.TopPairs {
			fmt.Fprintf(out, "  %dx  %s\n", p.Count, p.Pair)
		}
		fmt.Fprintln(out, "\nEither remove the duplication or accept it deliberately: `work_dot_bat check-duplication update [--allow-regression]`.")
		return 1
	case "update":
		b, err := UpdateDuplicationBaseline(root, hasFlag(rest, "--allow-regression"), git)
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 1
		}
		fmt.Fprintf(out, "Baseline updated: %d duplicated block(s) recorded.\n", b.TotalDuplicatedBlocks)
		return 0
	}
	fmt.Fprintf(errOut, "Unknown command: \"%s\"\nUsage: check-duplication <check|update> [--allow-regression]\n", cmd)
	return 1
}
