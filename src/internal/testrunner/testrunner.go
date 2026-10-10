package testrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"work_dot_bat/src/internal/projects"
)

const (
	LayerUltraLight = "ultra_light"
	LayerLight      = "light"
	LayerHeavy      = "heavy"
	LayerUltraHeavy = "ultra_heavy"
	LayerAll        = "all"
)

var AllLayers = []string{
	LayerUltraLight,
	LayerLight,
	LayerHeavy,
	LayerUltraHeavy,
}

// CommandRunner executes a command in dir, streaming stdout/stderr, and returns the exit code.
type CommandRunner func(ctx context.Context, cmd []string, dir string, stdout, stderr io.Writer) (int, error)

type Options struct {
	Project projects.Project
	Layer   string
	Dir     string
	Stdout  io.Writer
	Stderr  io.Writer
	Runner  CommandRunner
	Now     func() time.Time
}

// DefaultCommandRunner executes cmd via os/exec with stdout and stderr attached.
func DefaultCommandRunner(ctx context.Context, cmd []string, dir string, stdout, stderr io.Writer) (int, error) {
	if len(cmd) == 0 {
		return 0, nil
	}
	c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	c.Dir = dir
	c.Stdout = stdout
	c.Stderr = stderr
	c.Stdin = nil

	err := c.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	fmt.Fprintf(stderr, "Failed to execute %s: %v\n", cmd[0], err)
	return 1, err
}

// CommandsForLayer returns the commands configured for a given test layer in the project.
func CommandsForLayer(p projects.Project, layer string) (projects.CommandList, bool) {
	switch layer {
	case LayerUltraLight:
		return p.Tests.UltraLight, true
	case LayerLight:
		return p.Tests.Light, true
	case LayerHeavy:
		return p.Tests.Heavy, true
	case LayerUltraHeavy:
		return p.Tests.UltraHeavy, true
	default:
		return nil, false
	}
}

// TiersToRun returns the ordered list of tiers to run for a given target layer.
// Running a test at any level implies running all of the lower level tests first.
func TiersToRun(targetLayer string) ([]string, bool) {
	switch targetLayer {
	case LayerUltraLight:
		return []string{LayerUltraLight}, true
	case LayerLight:
		return []string{LayerUltraLight, LayerLight}, true
	case LayerHeavy:
		return []string{LayerUltraLight, LayerLight, LayerHeavy}, true
	case LayerUltraHeavy, LayerAll, "":
		return AllLayers, true
	default:
		return nil, false
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// Run executes the tests for the requested layer (running lower levels first) and returns the exit code.
func Run(ctx context.Context, opts Options) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Dir == "" {
		opts.Dir = opts.Project.Path
		if opts.Dir == "" {
			opts.Dir, _ = os.Getwd()
		}
	}
	if opts.Runner == nil {
		opts.Runner = DefaultCommandRunner
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	tiersToRun, ok := TiersToRun(opts.Layer)
	if !ok {
		fmt.Fprintf(opts.Stderr, "Unknown test layer %q. Allowed tiers: %s\n", opts.Layer, strings.Join(append(AllLayers, LayerAll), ", "))
		fmt.Fprintf(opts.Stderr, "Usage: work_dot_bat test [%s] [project=<name>]\n", strings.Join(append(AllLayers, LayerAll), "|"))
		return 2
	}

	for _, tier := range tiersToRun {
		cmds, ok := CommandsForLayer(opts.Project, tier)
		if !ok || len(cmds) == 0 {
			fmt.Fprintf(opts.Stdout, "No %s tests configured for project %q. Skipping.\n", tier, opts.Project.Name)
			continue
		}

		for _, cmd := range cmds {
			cmdStr := strings.Join(cmd, " ")
			fmt.Fprintf(opts.Stdout, "\n=== Running [%s] %s ===\n", tier, cmdStr)
			start := now()
			code, _ := opts.Runner(ctx, cmd, opts.Dir, opts.Stdout, opts.Stderr)
			elapsed := now().Sub(start)

			if code != 0 {
				fmt.Fprintf(opts.Stderr, "❌ [%s] %s failed with exit code %d (%s)\n", tier, cmdStr, code, formatDuration(elapsed))
				return code
			}
			fmt.Fprintf(opts.Stdout, "✅ [%s] %s passed (%s)\n", tier, cmdStr, formatDuration(elapsed))
		}
	}

	return 0
}
