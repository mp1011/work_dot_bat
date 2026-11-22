package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"work_dot_bat/src/internal/launcher"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/tools"
)

func runLauncher(args []string) int {
	regFile := projects.DefaultRegistryFile()
	if activeEnv != nil && activeEnv.registry != "" {
		regFile = activeEnv.registry
	}

	var proj projects.Project
	if activeProject != nil {
		proj = *activeProject
	} else {
		list, _, err := projects.LoadRegistry(regFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run: cannot load registry: %v\n", err)
			return 1
		}
		name := os.Getenv(repo.ProjectNameEnv)
		resolved, err := projects.ResolveProject(list, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "run: cannot resolve project: %v\n", err)
			return 1
		}
		proj = resolved
	}

	var stdin io.Reader = os.Stdin
	var stdout io.Writer = os.Stdout
	var stderr io.Writer = os.Stderr
	if activeEnv != nil {
		if activeEnv.stdin != nil {
			stdin = activeEnv.stdin
		}
		if activeEnv.stdout != nil {
			stdout = activeEnv.stdout
		}
		if activeEnv.stderr != nil {
			stderr = activeEnv.stderr
		}
	}

	in := bufio.NewReader(stdin)
	return launcher.Run(launcher.Options{
		Ask: func(prompt string) (string, bool) {
			fmt.Fprint(stdout, prompt)
			line, err := in.ReadString('\n')
			if err != nil && line == "" {
				return "", false
			}
			return strings.TrimRight(line, "\r\n"), true
		},
		Log:          func(msg string) { fmt.Fprintln(stdout, msg) },
		Project:      proj,
		RegistryFile: regFile,
		Stdin:        stdin,
		Stdout:       stdout,
		Stderr:       stderr,
	})
}

func init() {
	tools.Register("run", runLauncher)
}
