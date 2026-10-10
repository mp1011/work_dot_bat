package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/testrunner"
	"work_dot_bat/src/internal/tools"
)

func runTest(args []string) int {
	var proj projects.Project
	if activeProject != nil {
		proj = *activeProject
	} else {
		regFile := projects.DefaultRegistryFile()
		if activeEnv != nil && activeEnv.registry != "" {
			regFile = activeEnv.registry
		}
		list, _, err := projects.LoadRegistry(regFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "test: cannot load registry: %v\n", err)
			return 1
		}
		name := os.Getenv(repo.ProjectNameEnv)
		resolved, err := projects.ResolveProject(list, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "test: cannot resolve project: %v\n", err)
			return 1
		}
		proj = resolved
	}

	var stdout io.Writer = os.Stdout
	var stderr io.Writer = os.Stderr
	if activeEnv != nil {
		if activeEnv.stdout != nil {
			stdout = activeEnv.stdout
		}
		if activeEnv.stderr != nil {
			stderr = activeEnv.stderr
		}
	}

	layer := ""
	if len(args) == 1 {
		layer = args[0]
	} else if len(args) > 1 {
		fmt.Fprintf(stderr, "Usage: work_dot_bat test [ultra_light|light|heavy|ultra_heavy|all] [project=<name>]\n")
		return 2
	}

	return testrunner.Run(context.Background(), testrunner.Options{
		Project: proj,
		Layer:   layer,
		Dir:     proj.Path,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func init() {
	tools.Register("test", runTest)
}
