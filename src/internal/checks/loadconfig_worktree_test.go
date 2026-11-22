package checks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"work_dot_bat/src/internal/projects"
)

// worktreeFixture builds a main checkout (.git dir) and a linked worktree (.git file).
func worktreeFixture(t *testing.T) (main, tree string) {
	t.Helper()
	main = t.TempDir()
	tree = t.TempDir()
	wtGit := filepath.Join(main, ".git", "worktrees", "x")
	if err := os.MkdirAll(wtGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, ".git"), []byte("gitdir: "+filepath.ToSlash(wtGit)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return main, tree
}

func registerProject(t *testing.T, path string, checks []string) {
	t.Helper()
	regFile := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(projects.RegistryEnv, regFile)
	if err := projects.SaveRegistry(regFile, []projects.Project{{Name: "p", Path: path, Checks: checks}}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigWorktreeUsesMainRegistryEntry(t *testing.T) {
	main, tree := worktreeFixture(t)
	registerProject(t, main, []string{"monolith"})
	cfg, err := LoadConfig(tree)
	if err != nil {
		t.Fatalf("LoadConfig(worktree): %v", err)
	}
	if !reflect.DeepEqual(cfg.Checks, []string{"monolith"}) {
		t.Errorf("Checks = %v", cfg.Checks)
	}
}

func TestLoadConfigWorktreeUnregisteredMain(t *testing.T) {
	_, tree := worktreeFixture(t)
	registerProject(t, t.TempDir(), []string{"monolith"})
	_, err := LoadConfig(tree)
	if err == nil || !strings.Contains(err.Error(), "No work_dot_bat.config.json") {
		t.Errorf("err = %v", err)
	}
}

func TestLoadConfigPlainCheckoutRegistered(t *testing.T) {
	main, _ := worktreeFixture(t)
	registerProject(t, main, []string{"duplication"})
	cfg, err := LoadConfig(main)
	if err != nil || !reflect.DeepEqual(cfg.Checks, []string{"duplication"}) {
		t.Errorf("cfg=%v err=%v", cfg.Checks, err)
	}
}

func TestLoadConfigWorktreeOwnFileWins(t *testing.T) {
	main, tree := worktreeFixture(t)
	registerProject(t, main, []string{"monolith"})
	if err := os.WriteFile(filepath.Join(tree, "work_dot_bat.config.json"), []byte(`{"checks":["skills"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(tree)
	if err != nil || !reflect.DeepEqual(cfg.Checks, []string{"skills"}) {
		t.Errorf("cfg=%v err=%v", cfg.Checks, err)
	}
}
