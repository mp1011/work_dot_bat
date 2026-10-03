package checks

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitFunc runs git in dir and returns stdout.
type GitFunc = func(dir string, args ...string) (string, error)

func RealGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(ee.Stderr)))
	}
	return string(out), err
}

// RepoRoot is `git rev-parse --show-toplevel` from the current directory.
func RepoRoot() (string, error) {
	out, err := RealGit(".", "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}
