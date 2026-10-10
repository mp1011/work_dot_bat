package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"work_dot_bat/internal/projects"
)

type recordedCall struct {
	cmd []string
	dir string
}

func mockRunner(results map[string]int, recorded *[]recordedCall) CommandRunner {
	return func(ctx context.Context, cmd []string, dir string, stdout, stderr io.Writer) (int, error) {
		*recorded = append(*recorded, recordedCall{cmd: cmd, dir: dir})
		cmdStr := strings.Join(cmd, " ")
		fmt.Fprintf(stdout, "output of %s\n", cmdStr)
		if code, ok := results[cmdStr]; ok {
			if code != 0 {
				fmt.Fprintf(stderr, "error of %s\n", cmdStr)
			}
			return code, nil
		}
		return 0, nil
	}
}

func TestLayerDispatchUltraLight(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"lint", "-v"}},
			Light:      projects.CommandList{{"go", "test", "./..."}},
			Heavy:      projects.CommandList{{"go", "test", "-tags=heavy"}},
			UltraHeavy: projects.CommandList{{"gate.sh"}},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "ultra_light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 0 {
		t.Fatalf("expected code 0, got %d. stderr: %s", code, errb.String())
	}

	if len(recorded) != 1 {
		t.Fatalf("expected exactly 1 command run, got %d: %v", len(recorded), recorded)
	}

	if strings.Join(recorded[0].cmd, " ") != "lint -v" {
		t.Errorf("expected command 'lint -v', got %v", recorded[0].cmd)
	}
	if recorded[0].dir != proj.Path {
		t.Errorf("expected dir %s, got %s", proj.Path, recorded[0].dir)
	}

	// Verify higher tiers were NOT executed
	for _, call := range recorded {
		line := strings.Join(call.cmd, " ")
		if strings.Contains(line, "tags=heavy") || strings.Contains(line, "gate.sh") {
			t.Errorf("unexpected command run: %s", line)
		}
	}
}

func TestLayerHierarchy(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"cmd_ultra_light"}},
			Light:      projects.CommandList{{"cmd_light"}},
			Heavy:      projects.CommandList{{"cmd_heavy"}},
			UltraHeavy: projects.CommandList{{"cmd_ultra_heavy"}},
		},
	}

	// Running "light" should run ultra_light, then light
	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if len(recorded) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(recorded), recorded)
	}
	if recorded[0].cmd[0] != "cmd_ultra_light" || recorded[1].cmd[0] != "cmd_light" {
		t.Errorf("expected [cmd_ultra_light, cmd_light], got %v", recorded)
	}

	// Running "heavy" should run ultra_light, light, heavy
	recorded = nil
	code = Run(context.Background(), Options{
		Project: proj,
		Layer:   "heavy",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if len(recorded) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(recorded), recorded)
	}
	if recorded[0].cmd[0] != "cmd_ultra_light" || recorded[1].cmd[0] != "cmd_light" || recorded[2].cmd[0] != "cmd_heavy" {
		t.Errorf("expected [cmd_ultra_light, cmd_light, cmd_heavy], got %v", recorded)
	}
}

func TestMultiCommandLayer(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{
				{"npm", "run", "lint"},
				{"npx", "tsc", "--noEmit"},
			},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "ultra_light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}

	if len(recorded) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(recorded))
	}
	if strings.Join(recorded[0].cmd, " ") != "npm run lint" {
		t.Errorf("cmd 0: got %v", recorded[0].cmd)
	}
	if strings.Join(recorded[1].cmd, " ") != "npx tsc --noEmit" {
		t.Errorf("cmd 1: got %v", recorded[1].cmd)
	}
}

func TestFailureHandling(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			Light: projects.CommandList{
				{"go", "test", "./pkg/a"},
				{"go", "test", "./pkg/b"},
			},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(map[string]int{
		"go test ./pkg/a": 42,
	}, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}

	if len(recorded) != 1 {
		t.Fatalf("expected execution to halt after first failure, got %d calls", len(recorded))
	}
}

func TestLowerLevelFailureBlocksHigher(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"lint"}},
			Light:      projects.CommandList{{"unit"}},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(map[string]int{
		"lint": 1,
	}, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if len(recorded) != 1 {
		t.Fatalf("expected unit test not to run after lint failure, got %d calls", len(recorded))
	}
}

func TestUnconfiguredTier(t *testing.T) {
	proj := projects.Project{
		Name: "empty_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			// No ultra_light, no heavy
			Light: projects.CommandList{{"go", "test", "./..."}},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "ultra_light", // unconfigured
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 0 {
		t.Fatalf("expected code 0 for unconfigured tier, got %d", code)
	}
	if len(recorded) != 0 {
		t.Fatalf("expected 0 commands executed, got %d", len(recorded))
	}
	if !strings.Contains(out.String(), "ultra_light") || !strings.Contains(out.String(), "Skipping") {
		t.Errorf("expected skip notice in output, got %q", out.String())
	}
}

func TestInvalidTier(t *testing.T) {
	proj := projects.Project{Name: "test_proj", Path: "/test/path"}
	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "invalid_tier",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 2 {
		t.Fatalf("expected code 2 for invalid tier, got %d", code)
	}
	if len(recorded) != 0 {
		t.Fatalf("expected 0 commands executed, got %d", len(recorded))
	}

	errStr := errb.String()
	for _, expectedTier := range []string{"ultra_light", "light", "heavy", "ultra_heavy", "all"} {
		if !strings.Contains(errStr, expectedTier) {
			t.Errorf("expected error message to mention %q, got: %s", expectedTier, errStr)
		}
	}
}

func TestRunAllTiers(t *testing.T) {
	proj := projects.Project{
		Name: "all_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"lint"}},
			Light:      projects.CommandList{{"unit"}},
			Heavy:      projects.CommandList{{"integration"}},
			UltraHeavy: projects.CommandList{{"release_gate"}},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(nil, &recorded)
	var out, errb bytes.Buffer

	// Test explicit "all"
	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "all",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if len(recorded) != 4 {
		t.Fatalf("expected 4 commands executed, got %d", len(recorded))
	}

	expectedOrder := []string{"lint", "unit", "integration", "release_gate"}
	for i, want := range expectedOrder {
		got := strings.Join(recorded[i].cmd, " ")
		if got != want {
			t.Errorf("step %d: got %q, want %q", i, got, want)
		}
	}

	// Test default "" -> "all"
	recorded = nil
	out.Reset()
	errb.Reset()
	code = Run(context.Background(), Options{
		Project: proj,
		Layer:   "",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if len(recorded) != 4 {
		t.Fatalf("expected 4 commands executed for default, got %d", len(recorded))
	}
}

func TestRunAllStopsOnFailure(t *testing.T) {
	proj := projects.Project{
		Name: "all_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"lint"}},
			Light:      projects.CommandList{{"unit"}},
			Heavy:      projects.CommandList{{"integration"}},
			UltraHeavy: projects.CommandList{{"release_gate"}},
		},
	}

	var recorded []recordedCall
	runner := mockRunner(map[string]int{
		"unit": 3,
	}, &recorded)
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "all",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
	})

	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if len(recorded) != 2 {
		t.Fatalf("expected exactly 2 commands executed before halt, got %d: %v", len(recorded), recorded)
	}
}

func TestTimingReported(t *testing.T) {
	proj := projects.Project{
		Name: "test_proj",
		Path: "/test/path",
		Tests: projects.TestsConfig{
			Light: projects.CommandList{{"go", "test", "./..."}},
		},
	}

	fakeNow := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	runner := func(ctx context.Context, cmd []string, dir string, stdout, stderr io.Writer) (int, error) {
		fakeNow = fakeNow.Add(1250 * time.Millisecond)
		return 0, nil
	}
	var out, errb bytes.Buffer

	code := Run(context.Background(), Options{
		Project: proj,
		Layer:   "light",
		Dir:     proj.Path,
		Stdout:  &out,
		Stderr:  &errb,
		Runner:  runner,
		Now:     func() time.Time { return fakeNow },
	})

	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	outStr := out.String()
	if !strings.Contains(outStr, "1.25s") && !strings.Contains(outStr, "1250ms") && !strings.Contains(outStr, "passed") {
		t.Errorf("expected timing output in stdout, got: %s", outStr)
	}
}
