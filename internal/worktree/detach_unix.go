//go:build !windows

package worktree

import (
	"os/exec"
	"syscall"
)

// detach: its own session, so the child outlives this process.
func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
