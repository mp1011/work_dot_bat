//go:build windows

package proc

import (
	"os/exec"
	"strconv"
)

func prepare(cmd *exec.Cmd) {}

// killTree force-kills the process and every descendant (taskkill /T /F).
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
