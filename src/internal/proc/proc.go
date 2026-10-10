// Package proc runs child processes the way unattended callers need: stdin closed, output
// passed through, a timeout, a heartbeat, and a kill that takes the whole process tree.
package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// ErrTimedOut is returned when a process exceeded its timeout and was killed.
var ErrTimedOut = errors.New("timed out")

// Options configures Run.
type Options struct {
	Dir string
	// Env adds variables to the inherited environment.
	Env     map[string]string
	Timeout time.Duration // 0 = none
	// Heartbeat calls OnHeartbeat every interval while the process runs (0 = never).
	Heartbeat   time.Duration
	OnHeartbeat func(elapsed time.Duration)
}

// Run runs cmd with stdout/stderr passed through and stdin closed (unattended), returning
// its exit code. Cancelling ctx or exceeding the timeout kills the whole process tree, so an
// agent's children (shells, test runners, dev servers) never outlive it. A timeout returns
// ErrTimedOut; a start failure returns an error and code 1.
func Run(ctx context.Context, name string, args []string, o Options) (int, error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, name, args...)
	cmd.Dir = o.Dir
	if len(o.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range o.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	prepare(cmd)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to execute %s: %v\n", name, err)
		return 1, err
	}
	start := time.Now()
	timedOut := make(chan struct{})
	done := make(chan struct{})
	defer close(done)
	if o.Timeout > 0 {
		go func() {
			select {
			case <-time.After(o.Timeout):
				close(timedOut)
				cancel()
			case <-done:
			}
		}()
	}
	if o.Heartbeat > 0 && o.OnHeartbeat != nil {
		go func() {
			t := time.NewTicker(o.Heartbeat)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					o.OnHeartbeat(time.Since(start))
				case <-done:
					return
				}
			}
		}()
	}
	err := cmd.Wait()
	select {
	case <-timedOut:
		return 1, ErrTimedOut
	default:
	}
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code, nil
		}
		return 1, nil // killed by a signal: never a pass
	}
	return 1, err
}
