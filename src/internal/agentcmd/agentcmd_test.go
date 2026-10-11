package agentcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"work_dot_bat/src/internal/menu"
)

const root = "C:/repos/mm"

func defaults() Settings {
	return DefaultSettings()
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
	want := "/do-work-yolo p.md\n\nNote: do not mark the plan 'Ready for Review' — the automated test harness runs verification after you " +
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
	if got != want {
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
	if a, quit, ok := ChooseAgent("c", io, nil); a != "c" || quit || !ok || len(*out) != 0 {
		t.Errorf("already selected must not prompt: %v", *out)
	}
	for in, want := range map[string]string{"a": "a", "Antigravity": "a", "1": "a", "c": "c", "CLAUDE": "c", "2": "c"} {
		io, _ := scripted("zzz", in)
		if a, quit, ok := ChooseAgent("", io, nil); a != want || quit || !ok {
			t.Errorf("%q -> %q", in, a)
		}
	}
	io, out = scripted("bad", "q")
	if _, quit, ok := ChooseAgent("", io, nil); !quit || !ok {
		t.Error("q should quit")
	}
	wantOut := []string{"\nSelect Agent:", "  (a) Google Antigravity (Gemini 3.8 Flash)", "  (c) Claude Code (Sonnet)", "  (o) Claude Code (Opus)", "",
		"Choose agent (a/c/o): ", "Invalid agent selection. Please enter one of: a, c, o.", "Choose agent (a/c/o): ", "Exiting work script."}
	if !reflect.DeepEqual(*out, wantOut) {
		t.Errorf("got %q", *out)
	}
	io, _ = scripted()
	if _, _, ok := ChooseAgent("", io, nil); ok {
		t.Error("end of input should be not ok")
	}
}

func TestLoadDefinitions(t *testing.T) {
	dir := t.TempDir()
	if got := LoadDefinitions(dir); !reflect.DeepEqual(got, DefaultDefinitions()) {
		t.Errorf("no file should give built-ins: %+v", got)
	}
	os.WriteFile(filepath.Join(dir, CLIFile), []byte(`{"agents":[
	  {"id":"claude-sonnet","label":"Sonnet!","args":["--model","sonnet-x"]},
	  {"id":"gem","key":"g","label":"Gem","cmd":"gem","args":["-m","x"],"printArgs":["-p","{prompt}"],
	   "interactiveArgs":["{prompt}"],"session":{"reader":"agy","dir":"D"}},
	  {"id":"nokey","cmd":"n","session":{"reader":"claude"}},
	  {"cmd":"anon"}]}`), 0o644)
	defs := LoadDefinitions(dir)
	if len(defs) != 4 {
		t.Fatalf("want 3 built-ins + gem, got %d: %+v", len(defs), defs)
	}
	s := Settings{Defs: defs}
	if s.AgentLabel("c") != "Sonnet!" || s.Agent("claude").PrintArgs[0] != "--print" {
		t.Errorf("override should keep unspecified fields: %+v", s.Agent("c"))
	}
	if got := BuildCommand("c", CommandOptions{Prompt: "P"}, s); got.Args[1] != "sonnet-x" {
		t.Errorf("override args: %+v", got)
	}
	got := BuildCommand("g", CommandOptions{RepoRoot: root, Prompt: "P"}, s)
	if want := (Command{"gem", []string{"-m", "x", "-p", "P"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("custom agent: got %+v want %+v", got, want)
	}
	if d := s.Agent("gem"); d.Session.Reader != ReaderAgy || d.Session.Dir != "D" {
		t.Errorf("session data lost: %+v", d.Session)
	}
	if s.Agent("unknown").ID != "claude-sonnet" {
		t.Error("unknown agent should fall back to the first claude agent")
	}
	os.WriteFile(filepath.Join(dir, CLIFile), []byte(`{nope`), 0o644)
	if got := LoadDefinitions(dir); !reflect.DeepEqual(got, DefaultDefinitions()) {
		t.Error("invalid file should give built-ins")
	}
}

func TestModelsAreSeparateAgents(t *testing.T) {
	s := DefaultSettings()
	for agent, model := range map[string]string{"c": "sonnet", "claude": "sonnet", "o": "opus", "claude-opus": "opus"} {
		if got := BuildCommand(agent, CommandOptions{Prompt: "P"}, s); got.Args[1] != model {
			t.Errorf("%s: got %+v want model %s", agent, got, model)
		}
	}
	if s.Agent("o").ID != "claude-opus" || s.Agent("c").ID != "claude-sonnet" {
		t.Error("sonnet and opus must be distinct agents")
	}
}

func TestProjectSettingsAgentCLIFile(t *testing.T) {
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, CLIFile), []byte(`{"agents":[
	  {"id":"claude-opus","cmd":"claude2"},
	  {"id":"gem","key":"g","cmd":"gem","printArgs":["{prompt}"],"session":{"reader":"agy"}}]}`), 0o644)
	s := ProjectSettings(proj, proj)
	got := BuildCommand("o", CommandOptions{Prompt: "P"}, s)
	if want := (Command{"claude2", []string{"--model", "opus", "--permission-mode", "auto", "--print", "P"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
	if got := BuildCommand("g", CommandOptions{Prompt: "P"}, s); got.Cmd != "gem" {
		t.Errorf("got %+v", got)
	}
}

func TestChooseAgentCustom(t *testing.T) {
	defs := append(DefaultDefinitions(), Definition{ID: "gem", Key: "g", Label: "Gem"})
	io, _ := scripted("g")
	if a, _, ok := ChooseAgent("", io, defs); a != "g" || !ok {
		t.Errorf("got %q", a)
	}
	io, _ = scripted("4")
	if a, _, _ := ChooseAgent("", io, defs); a != "g" {
		t.Errorf("position: got %q", a)
	}
}

func TestRepoAgentCLIFileMatchesBuiltIns(t *testing.T) {
	got, _ := json.Marshal(LoadDefinitions(filepath.Join("..", "..", "..")))
	want, _ := json.Marshal(DefaultDefinitions())
	if string(got) != string(want) {
		t.Errorf("repo agent_cli.json drifted from the built-ins:\n got %s\nwant %s", got, want)
	}
}
