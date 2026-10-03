// Package repo: repo-root detection, per-project state paths, project activation, and a
// guard for tests that must not write into real local state.
package repo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	StateDirName   = ".work_dot_bat"
	ProjectRootEnv = "WORK_DOT_BAT_PROJECT_ROOT"
	ProjectNameEnv = "WORK_DOT_BAT_PROJECT_NAME"
)

func StateDir(root string) string   { return filepath.Join(root, StateDirName) }
func ClaimsDir(root string) string  { return filepath.Join(StateDir(root), "claims") }
func LedgerPath(root string) string { return filepath.Join(StateDir(root), "session_ledger.json") }
func LogsDir(root string) string    { return filepath.Join(StateDir(root), "logs") }

var gitdirLine = regexp.MustCompile(`(?m)^gitdir:\s*(.+)$`)

// FindMainRepoRoot resolves the main checkout's root from a worktree root. A directory
// .git (or none) means startDir is already the main root. A .git file points at
// <main>/.git/worktrees/<name>; the root is everything before the last "/.git".
func FindMainRepoRoot(startDir string) string {
	gitPath := filepath.Join(startDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil || info.IsDir() {
		return startDir
	}
	b, err := os.ReadFile(gitPath)
	if err != nil {
		return startDir
	}
	m := gitdirLine.FindSubmatch(b)
	if m == nil {
		return startDir
	}
	gitDir := strings.TrimSpace(string(m[1]))
	if !filepath.IsAbs(gitDir) && !strings.HasPrefix(gitDir, "/") {
		gitDir = filepath.Join(startDir, gitDir)
	}
	normalized := strings.ReplaceAll(gitDir, `\`, "/")
	if i := strings.LastIndex(normalized, "/.git"); i != -1 {
		return filepath.FromSlash(normalized[:i])
	}
	return startDir
}

// Activate makes the project "the" project for this process: chdir plus the env the engine reads.
func Activate(name, path string) error {
	if err := os.Chdir(path); err != nil {
		return err
	}
	os.Setenv(ProjectRootEnv, path)
	os.Setenv(ProjectNameEnv, name)
	return nil
}

// UnchangedGuard snapshots files and reports which changed (content or existence) since.
type UnchangedGuard struct {
	paths    []string
	baseline map[string]*string
}

func NewUnchangedGuard(paths ...string) *UnchangedGuard { return &UnchangedGuard{paths: paths} }

func readOrNil(p string) *string {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func (g *UnchangedGuard) Snapshot() {
	g.baseline = map[string]*string{}
	for _, p := range g.paths {
		g.baseline[p] = readOrNil(p)
	}
}

func (g *UnchangedGuard) Changed() []string {
	var out []string
	for _, p := range g.paths {
		a, b := readOrNil(p), g.baseline[p]
		if (a == nil) != (b == nil) || (a != nil && *a != *b) {
			out = append(out, p)
		}
	}
	return out
}
