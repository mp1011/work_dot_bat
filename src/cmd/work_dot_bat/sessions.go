package main

import (
	"os"
	"path/filepath"

	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/sessiontools"
	"work_dot_bat/src/internal/tools"
)

func sessionConfig() sessiontools.Config {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	return sessiontools.Config{Root: filepath.FromSlash(root), Out: os.Stdout, Err: os.Stderr}
}

func init() {
	tools.Register("analyze_session", func(args []string) int { return sessiontools.Analyze(sessionConfig(), args) })
	tools.Register("reclassify_sessions", func(args []string) int { return sessiontools.Reclassify(sessionConfig(), args) })
}
