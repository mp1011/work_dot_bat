package work

import (
	"context"
	"strings"
	"testing"

	"work_dot_bat/src/internal/immediate"
	"work_dot_bat/src/internal/taskmenu"
)

func recordFailure(t *testing.T, root string) {
	t.Helper()
	if err := immediate.Record(root, []immediate.Failure{{Name: "[light] go test", Command: "go test", Output: "FAIL"}}); err != nil {
		t.Fatal(err)
	}
}

// With a pending immediate plan the menu is skipped, the notice is shown and the agent works it.
func TestPendingImmediatePlanSkipsTheMenu(t *testing.T) {
	f := newFixture(t, "w", "", "c", "n", "s")
	recordFailure(t, f.root)
	picks := 0
	f.app.Pick = func() taskmenu.Result { picks++; return taskmenu.Result{Quit: true} }
	f.app.Run(context.Background())
	if picks != 1 {
		t.Errorf("the menu should only appear after the immediate plan (second iteration), got %d picks", picks)
	}
	if !strings.Contains(f.out.String(), "Immediate plan pending: picking agent_docs/plans/immediate.md") {
		t.Errorf("no notice:\n%s", f.out.String())
	}
	if len(f.agent) != 1 {
		t.Fatalf("agent runs: %+v\n%s", f.agent, f.out.String())
	}
	prompt := f.agent[0].Args[len(f.agent[0].Args)-1]
	if !strings.Contains(prompt, "agent_docs/plans/immediate.md") || !strings.Contains(prompt, "Do not run `plan-status.sh` on it") {
		t.Errorf("prompt %q", prompt)
	}
}

func TestDoneOrAbsentImmediatePlanLeavesTheMenuAlone(t *testing.T) {
	f := newFixture(t)
	picks := 0
	f.app.Pick = func() taskmenu.Result { picks++; return taskmenu.Result{Quit: true} }
	f.app.Run(context.Background())
	recordFailure(t, f.root)
	if err := immediate.Clear(f.root, []string{"[light] go test"}); err != nil {
		t.Fatal(err)
	}
	f.app.Run(context.Background())
	if picks != 2 || strings.Contains(f.out.String(), "Immediate plan pending") {
		t.Errorf("picks=%d\n%s", picks, f.out.String())
	}
}

