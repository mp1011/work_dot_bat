package main

import (
	"os"
	"path/filepath"

	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/tools"
	"work_dot_bat/src/internal/work"
)

// runWork is the interactive launcher, run in the activated project.
func runWork(args []string) int {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return work.Main(filepath.ToSlash(root), os.Getenv(repo.ProjectNameEnv), os.Stdin, os.Stdout, automatedTests)
}

// runPrepareForPush is the standalone pre-push workflow (human-invoked only).
func runPrepareForPush(args []string) int {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return work.PrepareForPush(filepath.ToSlash(root), os.Getenv(repo.ProjectNameEnv), os.Stdin, os.Stdout, automatedTests)
}

func init() {
	tools.Register("work", runWork)
	tools.Register("prepare_for_push", runPrepareForPush)
}
