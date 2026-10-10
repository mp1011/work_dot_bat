//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
	"time"
)

// prepare puts the child in its own process group so the whole tree can be signalled.
func prepare(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killTree asks the group to stop (SIGTERM) and force-kills it if it is still there 5s later.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.AfterFunc(5*time.Second, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
	return nil
}
