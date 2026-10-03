package main

import (
	"context"
	"os"
	"path/filepath"

	"work_dot_bat/internal/checks"
	"work_dot_bat/internal/repo"
)

func checksEnv() checks.Env { return checks.Env{Out: os.Stdout, Err: os.Stderr} }

func projectRootForChecks() string {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return filepath.ToSlash(root)
}

// automatedTests runs the project's whole gate in cwd and returns the failing check names; it is
// what the daemon, loop mode and the launcher's "run automated tests" use.
func automatedTests(ctx context.Context, cwd string) []string {
	return checksEnv().RunAutomatedTests(ctx, cwd, projectRootForChecks())
}
