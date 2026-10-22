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
		{"claude interactive", "5", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "P"}}},
		{"claude print", "5", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--print", "P"}}},
		{"claude session id", "5", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true, SessionID: "S"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--session-id", "S", "P"}}},
		{"agy interactive log", "1", CommandOptions{RepoRoot: root, Prompt: "P", Interactive: true, LogFile: "L"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--log-file", "L", "-i", "P"}}},
		{"agy print", "agy", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--print", "P"}}},
		{"claude ignores log file", "5", CommandOptions{RepoRoot: root, Prompt: "P", LogFile: "L"},
			Command{"claude", []string{"--model", "sonnet", "--permission-mode", "auto", "--print", "P"}}},
		{"agy ignores session id", "1", CommandOptions{RepoRoot: root, Prompt: "P", SessionID: "S"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--print", "P"}}},
		{"agy gemini 4 argon", "2", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"agy", []string{"--add-dir", root, "--model", "gemini-4-argon", "--effort", "medium", "--print", "P"}}},
		{"agy claude sonnet", "3", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"agy", []string{"--add-dir", root, "--model", "claude-sonnet-5-5", "--effort", "medium", "--print", "P"}}},
		{"agy claude opus", "4", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"agy", []string{"--add-dir", root, "--model", "claude-opus-5-5", "--effort", "medium", "--print", "P"}}},
		{"claude opus", "6", CommandOptions{RepoRoot: root, Prompt: "P"},
			Command{"claude", []string{"--model", "opus", "--permission-mode", "auto", "--print", "P"}}},
	}
	for _, c := range cases {
		if got := BuildCommand(c.agent, c.o, defaults()); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestBuildInvocationDefaultsAgyLogFromSessionName(t *testing.T) {
	c := BuildInvocation("1", root, "P", InvocationOptions{SessionName: "sess"}, defaults())
	want := []string{"--add-dir", root, "--model", "gemini-3.8-flash", "--effort", "medium", "--log-file", ".work_dot_bat/logs/sess.log", "--print", "P"}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("got %v", c.Args)
	}
	c = BuildInvocation("1", root, "P", InvocationOptions{SessionName: "sess", LogFile: "X"}, defaults())
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

	got = BuildRetryPrompt("p.md", "In Progress")
	want = "The previous execution of /do-work-yolo p.md ended with status \"In Progress\" instead of \"Ready for Review\".\n\n" +
		"Please continue working on this task using /do-work-yolo p.md. Complete all remaining requirements, ensure automated tests pass, " +
		"and set the task status to \"Ready for Review\" via `scripts/work_dot_bat/plan-status.sh set p.md \"Ready for Review\"`."
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
	if a, quit, ok := ChooseAgent("5", io, nil); a != "5" || quit || !ok || len(*out) != 0 {
		t.Errorf("already selected must not prompt: %v", *out)
	}
	for in, want := range map[string]string{
		"1": "1", "Antigravity": "1", "agy": "1",
		"2": "2", "argon": "2",
		"3": "3", "agy-sonnet": "3",
		"4": "4", "agy-opus": "4",
		"5": "5", "CLAUDE": "5", "claude": "5",
		"6": "6", "opus": "6",
	} {
		io, _ := scripted("zzz", in)
		if a, quit, ok := ChooseAgent("", io, nil); a != want || quit || !ok {
			t.Errorf("%q -> %q (want %q)", in, a, want)
		}
	}
	io, out = scripted("bad", "q")
	if _, quit, ok := ChooseAgent("", io, nil); !quit || !ok {
		t.Error("q should quit")
	}
	wantOut := []string{
		"\nSelect Agent:",
		"  (1) Google Antigravity (Gemini 3.8 Flash)",
		"  (2) Google Antigravity (Gemini 4 Argon)",
		"  (3) Google Antigravity (Claude Sonnet)",
		"  (4) Google Antigravity (Claude Opus)",
		"  (5) Claude Code (Sonnet)",
		"  (6) Claude Code (Opus)",
		"  (7) OpenAI Codex (GPT-6.1 Sol)",
		"  (8) OpenAI Codex (GPT-6 Astra)",
		"",
		"Choose agent (1/2/3/4/5/6/7/8): ",
		"Invalid agent selection. Please enter one of: 1, 2, 3, 4, 5, 6, 7, 8.",
		"Choose agent (1/2/3/4/5/6/7/8): ",
		"Exiting work script.",
	}
	if !reflect.DeepEqual(*out, wantOut) {
		t.Errorf("got %q want %q", *out, wantOut)
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
	  {"id":"gem","key":"9","label":"Gem","cmd":"gem","args":["-m","x"],"printArgs":["-p","{prompt}"],
	   "interactiveArgs":["{prompt}"],"session":{"reader":"agy","dir":"D"}},
	  {"id":"nokey","cmd":"n","session":{"reader":"claude"}},
	  {"cmd":"anon"}]}`), 0o644)
	defs := LoadDefinitions(dir)
	if len(defs) != 9 {
		t.Fatalf("want 8 built-ins + gem, got %d: %+v", len(defs), defs)
	}
	s := Settings{Defs: defs}
	if s.AgentLabel("5") != "Sonnet!" || s.Agent("claude").PrintArgs[0] != "--print" {
		t.Errorf("override should keep unspecified fields: %+v", s.Agent("5"))
	}
	if got := BuildCommand("5", CommandOptions{Prompt: "P"}, s); got.Args[1] != "sonnet-x" {
		t.Errorf("override args: %+v", got)
	}
	got := BuildCommand("9", CommandOptions{RepoRoot: root, Prompt: "P"}, s)
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
	for agent, model := range map[string]string{
		"5": "sonnet", "claude": "sonnet", "claude-sonnet": "sonnet",
		"6": "opus", "claude-opus": "opus",
		"1": "gemini-3.8-flash", "agy": "gemini-3.8-flash",
		"2": "gemini-4-argon", "argon": "gemini-4-argon",
		"3": "claude-sonnet-5-5", "agy-sonnet": "claude-sonnet-5-5",
		"4": "claude-opus-5-5", "agy-opus": "claude-opus-5-5",
	} {
		cmd := BuildCommand(agent, CommandOptions{Prompt: "P"}, s)
		found := false
		for i, arg := range cmd.Args {
			if arg == "--model" && i+1 < len(cmd.Args) && cmd.Args[i+1] == model {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: got args %+v want model %s", agent, cmd.Args, model)
		}
	}
	if s.Agent("6").ID != "claude-opus" || s.Agent("5").ID != "claude-sonnet" {
		t.Error("sonnet and opus must be distinct agents")
	}
	if s.Agent("3").ID != "agy-claude-sonnet" || s.Agent("5").ID != "claude-sonnet" {
		t.Error("agy claude-sonnet and claude-sonnet must be distinct agents")
	}
}

func TestProjectSettingsAgentCLIFile(t *testing.T) {
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, CLIFile), []byte(`{"agents":[
	  {"id":"claude-opus","cmd":"claude2"},
	  {"id":"gem","key":"9","cmd":"gem","printArgs":["{prompt}"],"session":{"reader":"agy"}}]}`), 0o644)
	s := ProjectSettings(proj, proj)
	got := BuildCommand("6", CommandOptions{Prompt: "P"}, s)
	if want := (Command{"claude2", []string{"--model", "opus", "--permission-mode", "auto", "--print", "P"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
	if got := BuildCommand("9", CommandOptions{Prompt: "P"}, s); got.Cmd != "gem" {
		t.Errorf("got %+v", got)
	}
}

func TestChooseAgentCustom(t *testing.T) {
	defs := append(DefaultDefinitions(), Definition{ID: "gem", Key: "9", Label: "Gem"})
	io, _ := scripted("9")
	if a, _, ok := ChooseAgent("", io, defs); a != "9" || !ok {
		t.Errorf("got %q", a)
	}
	io, _ = scripted("9")
	if a, _, _ := ChooseAgent("", io, defs); a != "9" {
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

func TestCodexCommands(t *testing.T) {
	s := DefaultSettings()
	for _, name := range []string{"8", "codex-astra", "astra", "gpt-6-astra"} {
		for _, interactive := range []bool{false, true} {
			got := BuildCommand(name, CommandOptions{Prompt: "P", Interactive: interactive}, s)
			args := []string{"--model", "gpt-6-astra", "--approve-for-me"}
			if !interactive {
				args = append(args, "exec")
			}
			want := Command{"codex", append(args, "P")}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s interactive=%v: got %+v want %+v", name, interactive, got, want)
			}
		}
	}
	for _, name := range []string{"7", "codex", "openai"} {
		got := BuildCommand(name, CommandOptions{Prompt: "P", LogFile: "L", SessionID: "unused"}, s)
		want := Command{"codex", []string{"--model", "gpt-6.1-sol", "--approve-for-me", "exec", "P", "--output-last-message", "L"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v want %+v", name, got, want)
		}
	}
	got := BuildCommand("codex", CommandOptions{Prompt: "P", Interactive: true, LogFile: "L"}, s)
	if want := (Command{"codex", []string{"--model", "gpt-6.1-sol", "--approve-for-me", "P"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("interactive: got %+v want %+v", got, want)
	}
	got = BuildCommand("codex", CommandOptions{Prompt: "P"}, s)
	if want := (Command{"codex", []string{"--model", "gpt-6.1-sol", "--approve-for-me", "exec", "P"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("no log: got %+v want %+v", got, want)
	}
}
