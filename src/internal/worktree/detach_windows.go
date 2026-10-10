//go:build windows

package worktree

import (
	"os/exec"
	"syscall"
)

// detach: new process group, no console window, so the child outlives this process.
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x08000000, HideWindow: true} // NEW_PROCESS_GROUP | CREATE_NO_WINDOW
}
