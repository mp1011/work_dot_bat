package main

import (
	"os"
	"path/filepath"

	"work_dot_bat/src/internal/daemon"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/tools"
	"work_dot_bat/src/internal/worktree"
)

func daemonConfig() daemon.Config {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return daemon.Config{
		Root: filepath.ToSlash(root), ProjectName: os.Getenv(repo.ProjectNameEnv),
		In: os.Stdin, Out: os.Stdout,
		RunTests: automatedTests,
		OpenEditor: func(path string) {
			worktree.RunCommand("code", []string{path}, worktree.RunOpts{})
		},
	}
}

func init() {
	tools.Register("auto_daemon", func([]string) int { return daemon.Main(daemonConfig()) })
	tools.Register("auto_status", func([]string) int { return daemon.Status(daemonConfig()) })
}
