package work

import (
	"context"
	"strings"
	"testing"

	"work_dot_bat/src/internal/agentcmd"
	"work_dot_bat/src/internal/taskmenu"
)

// pushRig scripts the gate's results and records everything the workflow launches.
func pushRig(t *testing.T, inputs ...string) (*fixture, *[][]string, *int) {
	f := newFixture(t, inputs...)
	var gates [][]string
	n := 0
	f.app.RunTests = func(layers ...string) []string {
		defer func() { n++ }()
		if n < len(gates) {
			return gates[n]
		}
		return nil
	}
	return f, &gates, &n
}

func gitCalls(f *fixture) []string {
	var out []string
	for _, c := range f.gitRun {
		out = append(out, strings.Join(c[1:], " "))
	}
	return out
}

func TestPushHappyPathRunsGateThenAgentThenPushes(t *testing.T) {
	f, _, ran := pushRig(t, "c", "n")
	agent, pushed := f.app.pushWorkflow(context.Background(), "")
	if !pushed || agent != "c" || *ran != 1 {
		t.Fatalf("%v %v gate runs %d\n%s", agent, pushed, *ran, f.out.String())
	}
	if len(f.agent) != 1 {
		t.Fatalf("agent runs %d", len(f.agent))
	}
	c := f.agent[0]
	wantPrompt := "/prepare-for-push Note: lint, typecheck, unit tests, and e2e (steps 3-6) already ran and passed via the full mechanical check suite immediately before this invocation. Skip those steps and go straight to steps 1, 2, 7, and 8."
	if c.Args[len(c.Args)-1] != wantPrompt || !contains(c.Args, "--print") {
		t.Errorf("the agent runs non-interactively with the skip-mechanical prompt: %+v", c)
	}
	if got := gitCalls(f); len(got) != 1 || got[0] != "push" {
		t.Errorf("git calls %v", got)
	}
	for _, want := range []string{"Mechanical checks passed. Running prepare-for-push via Claude Code (Sonnet) non-interactively...", "Options after prepare command:", "Running git push..."} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("missing %q:\n%s", want, f.out.String())
		}
	}
}

func TestPushFailingChecksSendTheAgentInInteractivelyAndNeverPushOnTheirOwn(t *testing.T) {
	f, gates, _ := pushRig(t, "c", "c")
	*gates = [][]string{{"ESLint", "Unit Tests"}, {"ESLint"}}
	f.app.GitOut = func(dir string, args ...string) (string, error) { return "", nil }
	_, pushed := f.app.pushWorkflow(context.Background(), "")
	if pushed {
		t.Fatal("two failed gate runs and a cancel must not push")
	}
	if len(f.agent) != 1 {
		t.Fatalf("agent runs %d", len(f.agent))
	}
	c := f.agent[0]
	wantPrompt := "The following automated check(s) failed during prepare-for-push: ESLint, Unit Tests. Please fix the failing checks so that all automated tests pass."
	if c.Args[len(c.Args)-1] != wantPrompt || contains(c.Args, "--print") {
		t.Errorf("the fix run is interactive: %+v", c)
	}
	for _, g := range gitCalls(f) {
		if g == "push" {
			t.Error("pushed despite the cancel")
		}
	}
	for _, want := range []string{"Mechanical checks failed:\n   - ESLint\n   - Unit Tests", "Launching Claude Code (Sonnet) interactively to fix failing checks...",
		"Re-running automated checks...", "Options after failing checks:", "Push cancelled."} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("missing %q:\n%s", want, f.out.String())
		}
	}
}

func TestPushRerunsTheAgentWhenAsked(t *testing.T) {
	// fail, fix → still failing → "y" re-run → passes → agent prepare → "n" → push.
	f, gates, _ := pushRig(t, "a", "y", "n")
	*gates = [][]string{{"lint"}, {"lint"}, nil}
	f.app.GitOut = func(dir string, args ...string) (string, error) { return "", nil }
	agent, pushed := f.app.pushWorkflow(context.Background(), "")
	if !pushed || agent != "a" || len(f.agent) != 3 {
		t.Fatalf("%v %v agents %d\n%s", agent, pushed, len(f.agent), f.out.String())
	}
	if f.agent[0].Cmd != "agy" {
		t.Errorf("%+v", f.agent[0])
	}
}

func TestPushCommitsAnAgentsChangesOnlyWhenTheTreeIsDirty(t *testing.T) {
	// Dirty after the fix run: add -A + commit with the failing check names.
	f, gates, _ := pushRig(t, "c", "n")
	*gates = [][]string{{"ESLint"}, nil}
	f.app.GitOut = func(dir string, args ...string) (string, error) { return " M src/a.ts\n", nil }
	f.app.pushWorkflow(context.Background(), "")
	got := gitCalls(f)
	want := []string{"add -A", "commit -m Fix failing checks before push: ESLint", "add -A", "commit -m prepare-for-push: apply automated fixes", "push"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("git calls %v", got)
	}

	// Clean tree: nothing staged or committed.
	g, gates2, _ := pushRig(t, "c", "n")
	*gates2 = [][]string{{"ESLint"}, nil}
	g.app.GitOut = func(dir string, args ...string) (string, error) { return "", nil }
	g.app.pushWorkflow(context.Background(), "")
	if got := gitCalls(g); len(got) != 1 || got[0] != "push" {
		t.Errorf("a clean tree must not produce empty commits: %v", got)
	}
}

func TestPushCancelAtThePrepareChoiceAndRerun(t *testing.T) {
	f, _, _ := pushRig(t, "c", "c")
	if _, pushed := f.app.pushWorkflow(context.Background(), ""); pushed {
		t.Error("cancel must not push")
	}
	if got := gitCalls(f); len(got) != 0 {
		t.Errorf("%v", got)
	}
	g, _, _ := pushRig(t, "c", "y", "n")
	_, pushed := g.app.pushWorkflow(context.Background(), "")
	if !pushed || len(g.agent) != 2 {
		t.Errorf("(y) re-runs the prepare command before continuing: pushed %v agents %d", pushed, len(g.agent))
	}
}

func TestPushRefusesWithoutAGateAndHonoursAQuitAtTheAgentChoice(t *testing.T) {
	f := newFixture(t)
	if _, pushed := f.app.pushWorkflow(context.Background(), ""); pushed || !strings.Contains(f.out.String(), "refusing to push without running the checks") {
		t.Errorf("%s", f.out.String())
	}
	g, _, _ := pushRig(t, "q")
	if _, pushed := g.app.pushWorkflow(context.Background(), ""); pushed || len(g.agent) != 0 {
		t.Error("q at the agent choice stops everything")
	}
	// An already-selected agent is not asked again.
	h, _, _ := pushRig(t, "n")
	if agent, pushed := h.app.pushWorkflow(context.Background(), "c"); !pushed || agent != "c" || strings.Contains(h.out.String(), "Choose agent") {
		t.Errorf("%v %v\n%s", agent, pushed, h.out.String())
	}
	var _ agentcmd.Command
}

func TestMenuPushUsesTheWorkflow(t *testing.T) {
	f, _, _ := pushRig(t, "p", "c", "n", "s")
	f.app.Pick = func() taskmenu.Result { return taskmenu.Result{} }
	picks := 0
	f.app.Pick = func() taskmenu.Result {
		picks++
		if picks > 1 {
			return taskmenu.Result{Quit: true}
		}
		return taskmenu.Result{}
	}
	f.app.Run(context.Background())
	if got := gitCalls(f); len(got) != 1 || got[0] != "push" {
		t.Errorf("%v\n%s", got, f.out.String())
	}
}

func TestPushEnforcesUltraHeavyTier(t *testing.T) {
	f := newFixture(t, "c", "n")
	var requestedLayers []string
	f.app.RunTests = func(layers ...string) []string {
		requestedLayers = append(requestedLayers, layers...)
		return nil
	}
	agent, pushed := f.app.pushWorkflow(context.Background(), "")
	if !pushed || agent != "c" {
		t.Fatalf("%v %v\n%s", agent, pushed, f.out.String())
	}
	if len(requestedLayers) == 0 || requestedLayers[0] != "ultra_heavy" {
		t.Errorf("expected pushWorkflow to request ultra_heavy tier, got %v", requestedLayers)
	}
}
