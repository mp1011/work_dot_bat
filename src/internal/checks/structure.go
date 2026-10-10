// Package checks is the mechanical gate: the repo checks (plan drift, monolith, duplication,
// structural duplication, source assertions, skills, audit), the project-structure doc
// regenerator, and the `run-checks` orchestration the pre-push hook, CI and the engine share.
package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"work_dot_bat/src/internal/projectconfig"
)

// The machine-owned region of the structure doc is between these markers (inserted on first
// run) and spans `## src` through the section before `## Quick mental model`.
const (
	BeginMarker = "<!-- BEGIN GENERATED FILE LISTING - do not edit by hand; run scripts/work_dot_bat/update-project-structure.sh -->"
	EndMarker   = "<!-- END GENERATED FILE LISTING -->"
	TodoSummary = "<!-- TODO: summarize -->"

	regionStartHeading = "## src"
	regionEndHeading   = "## Quick mental model"
)

// The generator is "parse and preserve": the set of files is mechanical (git ls-files) but the
// one-line summaries are human judgment, so it NEVER rewrites an existing summary. It keeps every
// heading, prose line and summary verbatim and only reconciles the set of file bullets:
//   - a file still on disk keeps its exact bullet line,
//   - a brand-new file in an already-listed directory gets one `TODO: summarize` bullet,
//   - a file no longer on disk has its bullet dropped (its summary follows a moved file),
//   - a file in a directory the doc doesn't list yet gets a freshly nested section.

func toPosix(p string) string { return strings.ReplaceAll(p, `\`, "/") }

func joinPath(base, rel string) string {
	if base == "" {
		return rel
	}
	if rel == "" {
		return base
	}
	return base + "/" + rel
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i != -1 {
		return p[:i]
	}
	return ""
}

// ancestorsOf is every ancestor of p from closest to the root (ending with ""), excluding p.
func ancestorsOf(p string) []string {
	var out []string
	cur := p
	for cur != "" {
		parent := dirOf(cur)
		out = append(out, parent)
		if parent == cur {
			break
		}
		cur = parent
	}
	return out
}

var (
	headingRE = regexp.MustCompile(`^#{2,4}\s+(.*\S)\s*$`)
	bulletRE  = regexp.MustCompile("^(\\s*)-\\s+(`.*)$")
	summaryRE = regexp.MustCompile("`\\s+-\\s+(.*)$")
)

// ParseHeadingPath: `## Top level` -> ""; `## src` -> "src"; `### src\audio` -> "src/audio".
func ParseHeadingPath(line string) (string, bool) {
	m := headingRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	if m[1] == "Top level" {
		return "", true
	}
	return toPosix(m[1]), true
}

// Bullet is a parsed `- ` line: its indent and backtick-delimited code spans. A normal bullet has
// one span; a grouped one lists several (`a.html`, `b.html` - summary).
type Bullet struct {
	Indent     int
	Tokens     []string
	HasSummary bool
}

func ParseBullet(line string) *Bullet {
	m := bulletRE.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	rest := m[2]
	var tokens []string
	for strings.HasPrefix(rest, "`") {
		end := strings.Index(rest[1:], "`")
		if end == -1 {
			break
		}
		end++
		tokens = append(tokens, rest[1:end])
		rest = rest[end+1:]
		if strings.HasPrefix(rest, ", `") {
			rest = rest[2:]
		} else {
			break
		}
	}
	if len(tokens) == 0 {
		return nil
	}
	return &Bullet{Indent: len(m[1]), Tokens: tokens, HasSummary: strings.HasPrefix(strings.TrimLeft(rest, " \t"), "-")}
}

type nodeKind int

const (
	kRaw nodeKind = iota
	kHeading
	kDir
	kGroup
	kFile
)

type node struct {
	idx         int
	line        string
	kind        nodeKind
	path        string // heading / dir path
	indent      int
	contextPath string
	hasSummary  bool
	isContainer bool
	paths       []string // group
	filePath    string   // file
}

type dirCtx struct {
	indent int
	dir    string
}

func parseRegion(region []string) []node {
	nodes := make([]node, 0, len(region))
	headingPath := ""
	var stack []dirCtx
	for idx, line := range region {
		if h, ok := ParseHeadingPath(line); ok {
			headingPath = h
			stack = stack[:0]
			nodes = append(nodes, node{idx: idx, line: line, kind: kHeading, path: h})
			continue
		}
		b := ParseBullet(line)
		if b == nil {
			nodes = append(nodes, node{idx: idx, line: line, kind: kRaw})
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= b.Indent {
			stack = stack[:len(stack)-1]
		}
		dirs := make([]string, len(stack))
		for i, s := range stack {
			dirs[i] = s.dir
		}
		contextPath := joinPath(headingPath, strings.Join(dirs, "/"))
		single := ""
		if len(b.Tokens) == 1 {
			single = b.Tokens[0]
		}
		switch {
		case single != "" && strings.HasSuffix(single, `\`):
			dirRel := toPosix(single[:len(single)-1])
			nodes = append(nodes, node{idx: idx, line: line, kind: kDir, indent: b.Indent, contextPath: contextPath,
				path: joinPath(contextPath, dirRel), hasSummary: b.HasSummary})
			stack = append(stack, dirCtx{b.Indent, dirRel})
		case len(b.Tokens) > 1:
			paths := make([]string, len(b.Tokens))
			for i, t := range b.Tokens {
				paths[i] = joinPath(contextPath, toPosix(t))
			}
			nodes = append(nodes, node{idx: idx, line: line, kind: kGroup, indent: b.Indent, contextPath: contextPath, paths: paths})
		default:
			nodes = append(nodes, node{idx: idx, line: line, kind: kFile, indent: b.Indent, contextPath: contextPath,
				filePath: joinPath(contextPath, toPosix(single))})
		}
	}
	// A directory bullet is a "container" when the next bullet/heading is indented deeper.
	for i := range nodes {
		if nodes[i].kind != kDir {
			continue
		}
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].kind == kRaw {
				continue
			}
			nodes[i].isContainer = nodes[j].kind != kHeading && nodes[j].indent > nodes[i].indent
			break
		}
	}
	return nodes
}

// resolveDiskPath: the doc occasionally misfiles a bullet under the wrong heading, so if the
// literal path isn't on disk, walk up the context looking for the same basename.
func resolveDiskPath(computed string, disk map[string]bool) (string, bool) {
	if disk[computed] {
		return computed, true
	}
	base := computed[strings.LastIndex(computed, "/")+1:]
	for _, a := range ancestorsOf(dirOf(computed)) {
		if c := joinPath(a, base); disk[c] {
			return c, true
		}
	}
	return "", false
}

func extractSummary(line string) (string, bool) {
	m := summaryRE.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

type container struct {
	pos         int
	isDir       bool
	ownIndent   int
	anchorIdx   int
	childIndent int
	hasChildInd bool
}

func computeAnchors(nodes []node, containers map[string]*container) {
	for _, c := range containers {
		anchor := nodes[c.pos].idx
		first, haveFirst := 0, false
		for j := c.pos + 1; j < len(nodes); j++ {
			n := nodes[j]
			if n.kind == kHeading {
				break
			}
			if n.kind == kRaw {
				continue
			}
			if c.isDir && n.indent <= c.ownIndent {
				break
			}
			anchor = n.idx
			if !haveFirst {
				first, haveFirst = n.indent, true
			}
		}
		c.anchorIdx = anchor
		if !c.hasChildInd {
			c.childIndent, c.hasChildInd = first, true
		}
	}
}

func deepestContainer(containers map[string]*container, file string) string {
	dir := dirOf(file)
	if _, ok := containers[dir]; ok {
		return dir
	}
	for _, a := range ancestorsOf(dir) {
		if _, ok := containers[a]; ok {
			return a
		}
	}
	return ""
}

type entry struct {
	relSegs []string
	file    string
}

func renderSubtree(entries []entry, indent int, summaryByBase map[string]*string) (lines, files []string) {
	var fileNames []string
	dirGroups := map[string][]entry{}
	var dirOrder []string
	byName := map[string]entry{}
	for _, e := range entries {
		byName[e.relSegs[0]] = e
		if len(e.relSegs) == 1 {
			fileNames = append(fileNames, e.relSegs[0])
			continue
		}
		head := e.relSegs[0]
		if _, ok := dirGroups[head]; !ok {
			dirOrder = append(dirOrder, head)
		}
		dirGroups[head] = append(dirGroups[head], entry{e.relSegs[1:], e.file})
	}
	seen := map[string]bool{}
	var names []string
	for _, n := range append(append([]string{}, fileNames...), dirOrder...) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)
	pad := strings.Repeat(" ", indent)
	for _, name := range names {
		if g, ok := dirGroups[name]; ok {
			lines = append(lines, fmt.Sprintf("%s- `%s\\`", pad, name))
			subLines, subFiles := renderSubtree(g, indent+2, summaryByBase)
			lines = append(lines, subLines...)
			files = append(files, subFiles...)
			continue
		}
		summary := TodoSummary
		if carried := summaryByBase[name]; carried != nil {
			summary = *carried
		}
		lines = append(lines, fmt.Sprintf("%s- `%s` - %s", pad, name, summary))
		files = append(files, byName[name].file)
	}
	return lines, files
}

func pruneEmptyContainers(lines []string) []string {
	cur := lines
	for {
		nodes := parseRegion(cur)
		remove := map[int]bool{}
		for i, n := range nodes {
			if n.kind != kHeading && n.kind != kDir {
				continue
			}
			hasChild := false
			for j := i + 1; j < len(nodes); j++ {
				next := nodes[j]
				if next.kind == kHeading {
					break
				}
				if n.kind == kDir && next.kind != kRaw && next.indent <= n.indent {
					break
				}
				if next.kind == kFile || next.kind == kGroup || (next.kind == kDir && !remove[next.idx]) {
					hasChild = true
					break
				}
			}
			if !hasChild {
				remove[n.idx] = true
			}
		}
		if len(remove) == 0 {
			break
		}
		var next []string
		for idx, l := range cur {
			if remove[idx] {
				continue
			}
			if strings.TrimSpace(l) == "" && len(next) > 0 && strings.TrimSpace(next[len(next)-1]) == "" {
				continue
			}
			next = append(next, l)
		}
		if len(next) == len(cur) {
			break
		}
		cur = next
	}
	return cur
}

// Dropped is a bullet whose file is gone.
type Dropped struct{ Path, Line string }

type insertion struct {
	indent int
	lines  []string
}

// ReconcileRegion owns the whole listing: every tracked file ends up represented exactly once.
// Existing summaries are kept verbatim; new files get a TODO placeholder; deleted-file bullets
// are dropped; files in directories the doc doesn't list get nested sections under the nearest
// existing container.
func ReconcileRegion(region, diskFiles []string) (lines, added []string, dropped []Dropped) {
	disk := map[string]bool{}
	for _, f := range diskFiles {
		disk[f] = true
	}
	nodes := parseRegion(region)
	consumed := map[string]bool{}
	dropIdx := map[int]bool{}
	// Summaries of dropped bullets by basename, so a moved file carries its summary; ambiguous
	// basenames are disabled (nil).
	summaryByBase := map[string]*string{}
	seenBase := map[string]bool{}

	containers := map[string]*container{}
	for pos, n := range nodes {
		if n.kind == kDir {
			containers[n.path] = &container{pos: pos, isDir: true, ownIndent: n.indent, anchorIdx: n.idx, childIndent: n.indent + 2, hasChildInd: true}
		}
	}
	// Headings win on a path collision, so src files stay in the detailed section.
	for pos, n := range nodes {
		if n.kind == kHeading {
			containers[n.path] = &container{pos: pos, ownIndent: -1, anchorIdx: n.idx}
		}
	}
	if _, ok := containers[""]; !ok {
		containers[""] = &container{pos: 0, ownIndent: -1, anchorIdx: max(0, len(region)-1), childIndent: 0, hasChildInd: true}
	}
	computeAnchors(nodes, containers)

	for _, n := range nodes {
		switch n.kind {
		case kFile:
			resolved, ok := resolveDiskPath(n.filePath, disk)
			if !ok {
				dropIdx[n.idx] = true
				dropped = append(dropped, Dropped{n.filePath, n.line})
				base := n.filePath[strings.LastIndex(n.filePath, "/")+1:]
				if seenBase[base] {
					summaryByBase[base] = nil
				} else if s, ok := extractSummary(n.line); ok {
					summaryByBase[base] = &s
				} else {
					summaryByBase[base] = nil
				}
				seenBase[base] = true
				continue
			}
			consumed[resolved] = true
		case kGroup:
			for _, p := range n.paths {
				if resolved, ok := resolveDiskPath(p, disk); ok {
					consumed[resolved] = true
				}
			}
		}
	}

	byContainer := map[string][]entry{}
	var containerOrder []string
	sorted := append([]string{}, diskFiles...)
	sort.Strings(sorted)
	for _, file := range sorted {
		if consumed[file] {
			continue
		}
		c := deepestContainer(containers, file)
		rel := file
		if c != "" {
			rel = file[len(c)+1:]
		}
		if _, ok := byContainer[c]; !ok {
			containerOrder = append(containerOrder, c)
		}
		byContainer[c] = append(byContainer[c], entry{strings.Split(rel, "/"), file})
	}

	insertions := map[int][]insertion{}
	for _, c := range containerOrder {
		ct := containers[c]
		base := 0
		if ct.hasChildInd {
			base = ct.childIndent
		}
		rendered, files := renderSubtree(byContainer[c], base, summaryByBase)
		added = append(added, files...)
		insertions[ct.anchorIdx] = append(insertions[ct.anchorIdx], insertion{base, rendered})
	}

	var out []string
	for idx, l := range region {
		if !dropIdx[idx] {
			out = append(out, l)
		}
		if extra := insertions[idx]; extra != nil {
			// The deepest-indented block first, so its bullets stay nested under the anchor
			// before a shallower bullet pops the context back up (stable for equal indents).
			sort.SliceStable(extra, func(i, j int) bool { return extra[i].indent > extra[j].indent })
			for _, g := range extra {
				out = append(out, g.lines...)
			}
		}
	}
	sort.Strings(added)
	return pruneEmptyContainers(out), added, dropped
}

// FindUnsummarizedFiles lists the files whose bullet still says `TODO: summarize`. The
// generator must not report success while any remain, or the debt accrues silently.
func FindUnsummarizedFiles(doc string) []string {
	lines := splitLines(doc)
	begin, end := indexOf(lines, BeginMarker), indexOf(lines, EndMarker)
	region := lines
	if begin != -1 && end != -1 && begin < end {
		region = lines[begin+1 : end]
	}
	var out []string
	for _, n := range parseRegion(region) {
		if n.line == "" || !strings.Contains(n.line, TodoSummary) {
			continue
		}
		switch n.kind {
		case kFile:
			out = append(out, n.filePath)
		case kGroup:
			out = append(out, n.paths...)
		}
	}
	return out
}

var lineSplitRE = regexp.MustCompile(`\r?\n`)

func splitLines(s string) []string { return lineSplitRE.Split(s, -1) }

func indexOf(lines []string, s string) int {
	for i, l := range lines {
		if l == s {
			return i
		}
	}
	return -1
}

// RegenerateDoc locates (or inserts) the markers and reconciles the region.
func RegenerateDoc(doc string, diskFiles []string) (newText string, added []string, dropped []Dropped, err error) {
	eol := "\n"
	if strings.Contains(doc, "\r\n") {
		eol = "\r\n"
	}
	lines := splitLines(doc)
	begin, end := indexOf(lines, BeginMarker), indexOf(lines, EndMarker)
	var pre, region, post []string
	if begin != -1 && end != -1 && begin < end {
		pre, region, post = lines[:begin+1], lines[begin+1:end], lines[end:]
	} else {
		start, stop := -1, -1
		for i, l := range lines {
			if start == -1 && strings.TrimSpace(l) == regionStartHeading {
				start = i
			}
			if stop == -1 && strings.TrimSpace(l) == regionEndHeading {
				stop = i
			}
		}
		if start == -1 || stop == -1 || start >= stop {
			return "", nil, nil, fmt.Errorf("Could not locate the region between %q and %q.", regionStartHeading, regionEndHeading)
		}
		pre = append(append([]string{}, lines[:start]...), BeginMarker)
		region = lines[start:stop]
		post = append([]string{EndMarker}, lines[stop:]...)
	}
	body, added, dropped := ReconcileRegion(region, diskFiles)
	all := append(append(append([]string{}, pre...), body...), post...)
	return strings.Join(all, eol), added, dropped, nil
}

// StructureConfig is where the structure doc lives and which folders it lists.
type StructureConfig struct {
	Doc  string
	Dirs []string
}

// Defaults match the layout this tool was extracted from.
const DefaultStructureDoc = "agent_docs/03_project_structure.md"

var defaultStructureDirs = []string{"src", "e2e", "scripts", ".tools"}

// ReadStructureConfig reads the optional per-repo overrides (projectStructureDoc,
// projectStructureDirs); an unreadable config falls back to the defaults.
func ReadStructureConfig(root string) StructureConfig {
	cfg := StructureConfig{Doc: DefaultStructureDoc, Dirs: defaultStructureDirs}
	loaded := projectconfig.Load(root)
	if loaded.Config == nil {
		return cfg
	}
	if d, ok := loaded.Config["projectStructureDoc"].(string); ok {
		cfg.Doc = d
	}
	if list, ok := loaded.Config["projectStructureDirs"].([]any); ok {
		cfg.Dirs = nil
		for _, v := range list {
			if s, ok := v.(string); ok {
				cfg.Dirs = append(cfg.Dirs, s)
			}
		}
	}
	return cfg
}

// ListGitFiles is the tracked files the doc lists: no markdown, no node_modules, not the managed
// scripts (copies owned by work_dot_bat), and nothing deleted on disk but not yet staged.
func ListGitFiles(root string, dirs []string, git func(dir string, args ...string) (string, error)) ([]string, error) {
	out, err := git(root, append([]string{"ls-files", "--"}, dirs...)...)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out, "\n") {
		f = strings.TrimSpace(f)
		if f == "" || strings.HasSuffix(f, ".md") || strings.HasSuffix(f, ".plan.md") || strings.Contains(f, "/node_modules/") ||
			strings.HasPrefix(f, "scripts/work_dot_bat/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}
