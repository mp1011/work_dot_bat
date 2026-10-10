package main

import (
	"context"
	"os"
	"path/filepath"

	"work_dot_bat/src/internal/checks"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
)

func checksEnv() checks.Env {
	e := checks.Env{Out: os.Stdout, Err: os.Stderr}
	if activeEnv != nil {
		if activeEnv.stdout != nil {
			e.Out = activeEnv.stdout
		}
		if activeEnv.stderr != nil {
			e.Err = activeEnv.stderr
		}
		e.RegistryFile = activeEnv.registry
	}
	if activeProject != nil {
		e.Projects = []projects.Project{*activeProject}
	}
	return e
}

func projectRootForChecks() string {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return filepath.ToSlash(root)
}

// automatedTests runs the project's gate in cwd and returns the failing check names; it is
// what the daemon, loop mode, prepare-for-push and the launcher's test options use.
func automatedTests(ctx context.Context, cwd string, targetLayers ...string) []string {
	return checksEnv().RunAutomatedTests(ctx, cwd, projectRootForChecks(), targetLayers...)
}
