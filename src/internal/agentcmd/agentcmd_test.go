package agentcmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"work_dot_bat/src/internal/menu"
)

const root = "C:/repos/mm"

func defaults() Settings {
	return ResolveSettings(filepath.Join(os.TempDir(), "no-such-settings.json"), nil)
}

func TestBuildCommandDefaults(t *testing.T) {
	cases := []struct {
		name  string
		agent string
		o     CommandOptions
		want  Command
	}{
		{"claude interactive", "c", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "P"}}},
		{"claude print", "c", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--print", "P"}}},
		{"claude session id", "c", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true, SessionID: "S"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--session-id", "S", "P"}}},
		{"agy interactive log", "a", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true, LogFile: "L"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--log-file", "L", "-i", "P"}}},
		{"agy print", "agy", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--print", "P"}}},
		{"claude ignores log file", "c", CommandOptions{RepoRoot: root, Prompt: "P", LogFile: "L"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--print", "P"}}},
		{"agy ignores session id", "a", CommandOptions{RepoRoot: root, Prompt: "P", SessionID: "S"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--print", "P"}}},
	}
	for _, c := range cases {
		if got := BuildCommand(c.agent, c.o, defaults()); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestSettingsLayering(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(file, []byte(`{"claude":{"model":"opus"},"agy":{"effort":"high"}}`), 0o644)
	got := ResolveSettings(file, map[string]any{"claude": map[string]any{"model": "haiku"}})
	if !reflect.DeepEqual(got.Claude, AgentSettings{"cmd": "claude", "model": "haiku", "permissionMode": "auto"}) {
		t.Errorf("claude: %v", got.Claude)
	}
	if !reflect.DeepEqual(got.Agy, AgentSettings{"cmd": "agy", "model": "gemini-3.8-flash", "effort": "high"}) {
		t.Errorf("agy: %v", got.Agy)
	}
	os.WriteFile(file, []byte(`{nope`), 0o644)
	if got := ResolveSettings(file, nil); !reflect.DeepEqual(got, DefaultSettings()) {
		t.Errorf("unreadable file should give defaults: %v", got)
	}
}

func TestSettingsEnvAndProjectOverride(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(file, []byte(`{"agy":{"model":"m2"}}`), 0o644)
	t.Setenv(SettingsEnv, file)
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "work_dot_bat.config.json"),
		[]byte(`{"agents":{"claude":{"permissionMode":"plan"}}}`), 0o644)
	s := ProjectSettings(proj, proj)
	if s.Claude.str("permissionMode") != "plan" || s.Claude.str("model") != "sonnet" || s.Agy.str("model") != "m2" {
		t.Errorf("got %+v", s)
	}
	// no config at all: tool-level only
	if s := ProjectSettings(t.TempDir(), t.TempDir()); s.Agy.str("model") != "m2" {
		t.Errorf("got %+v", s)
	}
}

func TestBuildInvocationDefaultsAgyLogFromSessionName(t *testing.T) {
	c := BuildInvocation("a", root, "P", InvocationOptions{SessionName: "sess"}, defaults())
	want := []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--log-file", ".work_dot_bat/logs/sess.log", "--print", "P"}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("got %v", c.Args)
	}
	c = BuildInvocation("a", root, "P", InvocationOptions{SessionName: "sess", LogFile: "X"}, defaults())
	if c.Args[7] != "X" {
		t.Errorf("explicit log file should win: %v", c.Args)
	}
}

func TestPrompts(t *testing.T) {
	if got := BuildDoWorkYoloPrompt("p.md", false); got != "/do-work-yolo p.md" {
		t.Error(got)
	}
	want := "/do-work-yolo p.md\n\nNote: the launcher that spawned you already holds the plan/branch claim and " +
		"releases it when you exit. Skip STEP ZERO's acquire and do not run `plan-claim.sh release`; " +
		"still run the `concurrent-check` read. " +
		"Do not mark the plan 'Ready for Review' — the automated test harness runs verification after you " +
		"exit and sets Ready for Review only after all checks pass."
	if got := BuildDoWorkYoloPrompt("p.md", true); got != want {
		t.Error(got)
	}

	got := BuildFailureFollowupPrompt("p.md", "PLAN BODY", []string{"lint", "unit"}, 1, 3)
	want = "Automated tests failed after working on the plan at p.md (attempt 1 of 3).\n\nFailing checks:\n- lint\n- unit\n\n" +
		"Here is the plan file (p.md) for context:\n\nPLAN BODY\n\n" +
		"Investigate the failure(s) — re-run the specific failing check(s) yourself to see the full error output — and fix them. Do not commit or push."
	if got != want {
		t.Error(got)
	}

	got = BuildRetryPrompt("p.md", "In Progress", false)
	want = "The previous execution of /do-work-yolo p.md ended with status \"In Progress\" instead of \"Ready for Review\".\n\n" +
		"Please continue working on this task using /do-work-yolo p.md. Complete all remaining requirements, ensure automated tests pass, " +
		"and set the task status to \"Ready for Review\" via `scripts/work_dot_bat/plan-status.sh set p.md \"Ready for Review\"`."
	if got != want {
		t.Error(got)
	}
	got = BuildRetryPrompt("p.md", "In Progress", true)
	if !strings.HasSuffix(got, "`.\n\nNote: work.bat already holds the plan/branch claim and releases it when you exit. Skip STEP ZERO's acquire and do not run `plan-claim.sh release`; still run the `concurrent-check` read.") {
		t.Error(got)
	}
}

func TestGeneralAgentPrompt(t *testing.T) {
	var warned []string
	warn := func(s string) { warned = append(warned, s) }
	if got := BuildGeneralAgentPrompt("plans/x.md", "", "", warn); got != "Read the plan file at plans/x.md for context." || warned != nil {
		t.Errorf("%q %v", got, warned)
	}
	dir := t.TempDir()
	got := BuildGeneralAgentPrompt("plans/x.md", "Do it.", dir, warn)
	if got != "Read the plan file at plans/x.md for context.\n\nDo it." || len(warned) != 1 || warned[0] != "\n[WARN] Could not find plan file at plans/x.md" {
		t.Errorf("%q %v", got, warned)
	}
	os.MkdirAll(filepath.Join(dir, "plans"), 0o755)
	os.WriteFile(filepath.Join(dir, "plans", "x.md"), nil, 0o644)
	BuildGeneralAgentPrompt("plans/x.md", "", dir, warn)
	if len(warned) != 1 {
		t.Error("warned for an existing file")
	}
}

func scripted(inputs ...string) (menu.IO, *[]string) {
	var out []string
	return menu.IO{
		Ask: func(p string) (string, bool) {
			out = append(out, p)
			if len(inputs) == 0 {
				return "", false
			}
			in := inputs[0]
			inputs = inputs[1:]
			return in, true
		},
		Log: func(m string) { out = append(out, m) },
	}, &out
}

func TestChooseAgent(t *testing.T) {
	io, out := scripted()
	if a, quit, ok := ChooseAgent("c", io); a != "c" || quit || !ok || len(*out) != 0 {
		t.Errorf("already selected must not prompt: %v", *out)
	}
	for in, want := range map[string]string{"a": "a", "Antigravity": "a", "1": "a", "c": "c", "CLAUDE": "c", "2": "c"} {
		io, _ := scripted("zzz", in)
		if a, quit, ok := ChooseAgent("", io); a != want || quit || !ok {
			t.Errorf("%q -> %q", in, a)
		}
	}
	io, out = scripted("bad", "q")
	if _, quit, ok := ChooseAgent("", io); !quit || !ok {
		t.Error("q should quit")
	}
	wantOut := []string{"\nSelect Agent:", "  (a) Google Antigravity (Gemini 3.8 Flash)", "  (c) Claude Code (Sonnet)\n",
		"Choose agent (a/c): ", `Invalid agent selection. Please enter "a" for Antigravity or "c" for Claude.`, "Choose agent (a/c): ", "Exiting work script."}
	if !reflect.DeepEqual(*out, wantOut) {
		t.Errorf("got %q", *out)
	}
	io, _ = scripted()
	if _, _, ok := ChooseAgent("", io); ok {
		t.Error("end of input should be not ok")
	}
}
