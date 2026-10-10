package work

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"work_dot_bat/internal/planclaims"
)

// New wires an App to the real process: stdin for prompts, out for output, real git, claims and
// agent processes.
func New(root, projectName string, in io.Reader, out io.Writer) App {
	reader := bufio.NewReader(in)
	return App{
		Root: root, ProjectName: projectName, Out: out,
		IsTTY: isTerminal(out),
		Ask: func(prompt string) (string, bool) {
			fmt.Fprint(out, prompt)
			line, err := reader.ReadString('\n')
			if err != nil && line == "" {
				return "", false
			}
			return strings.TrimRight(line, "\r\n"), true
		},
		ClaimEnv: planclaims.DefaultEnv(),
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// Main runs the launcher until the user quits. A SIGTERM cancels the context, which kills the
// agent and lets the deferred claim release run; Ctrl-C belongs to the agent while it runs.
func Main(root, projectName string, in io.Reader, out io.Writer, runTests func(ctx context.Context, cwd string, layers ...string) []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	app := New(root, projectName, in, out)
	if runTests != nil {
		app.RunTests = func(layers ...string) []string { return runTests(ctx, root, layers...) }
	}
	return app.Run(ctx)
}
