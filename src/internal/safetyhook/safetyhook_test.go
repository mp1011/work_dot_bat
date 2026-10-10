package safetyhook

import (
	"encoding/json"
	"strings"
	"testing"
)

func call(cmd string) []byte {
	b, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd}})
	return b
}

func TestCheckBlocksTheSameCommandsWithTheSameMessages(t *testing.T) {
	blockPush := []string{
		"node src/prepare_for_push.js",
		"src\\prepare_for_push.bat",
		"PREPARE_FOR_PUSH.JS --yes",
		`node src/launch.mjs prepare_for_push project=x`,
		`node src/launch.mjs" prepare_for_push`,
		"work_dot_bat prepare_for_push project=x",
		"work_dot_bat.exe prepare_for_push",
	}
	for _, c := range blockPush {
		if code, msg := Check(call(c)); code != 2 || msg != PushMessage {
			t.Errorf("%q: %d %q", c, code, msg)
		}
	}
	blockWork := []string{
		"node src/work.js",
		"work.bat",
		"cd x && work.bat",
		`node src/launch.mjs work project=x`,
		"work_dot_bat work project=x",
		"work_dot_bat.exe work",
	}
	for _, c := range blockWork {
		if code, msg := Check(call(c)); code != 2 || msg != WorkMessage {
			t.Errorf("%q: %d %q", c, code, msg)
		}
	}
	allow := []string{
		"ls", "git status", "go test ./...", "cat agent_docs/plans/x.md",
		"scripts/work_dot_bat/plan-status.sh set x Done", // its own folder name must not trip it
		"work_dot_bat auto_status project=x",
		"work_dot_bat worktree-info",
		"echo network", "git log --oneline", "rm -rf networking.js",
	}
	for _, c := range allow {
		if code, msg := Check(call(c)); code != 0 || msg != "" {
			t.Errorf("%q should be allowed: %d %q", c, code, msg)
		}
	}
	// Anything that is not a parseable hook call is allowed.
	for _, in := range []string{"", "not json", "{}", `{"tool_input":{}}`, `{"tool_input":{"command":5}}`} {
		if code, _ := Check([]byte(in)); code != 0 {
			t.Errorf("%q: %d", in, code)
		}
	}
	// A prepare_for_push mention wins over work when both appear.
	if _, msg := Check(call("work.bat; prepare_for_push.bat")); msg != PushMessage {
		t.Errorf("%q", msg)
	}
}

func TestMergeBuildsFullSettingsFromNothing(t *testing.T) {
	m, err := Merge(nil)
	if err != nil || !m.Changed || len(m.Added) != len(DenyRules)+1 {
		t.Fatalf("%+v %v", m, err)
	}
	var got struct {
		Permissions struct{ Deny []string } `json:"permissions"`
		Hooks       struct {
			PreToolUse []struct {
				Matcher string
				Hooks   []struct{ Type, Shell, Command string }
			}
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(m.JSON), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Permissions.Deny, ",") != strings.Join(DenyRules, ",") {
		t.Errorf("%v", got.Permissions.Deny)
	}
	h := got.Hooks.PreToolUse
	if len(h) != 1 || h[0].Matcher != "Bash|PowerShell" || h[0].Hooks[0].Command != HookCommand || h[0].Hooks[0].Shell != "bash" || h[0].Hooks[0].Type != "command" {
		t.Errorf("%+v", h)
	}
	if !strings.HasSuffix(m.JSON, "}\n") || !strings.Contains(m.JSON, "\n  \"permissions\": {\n    \"deny\": [\n      \"Bash(git commit*)\"") {
		t.Errorf("format:\n%s", m.JSON)
	}
}

func TestMergeIsIdempotentAndKeepsEverythingElseInPlace(t *testing.T) {
	existing := `{
  "model": "opus",
  "permissions": {
    "allow": ["Bash(ls*)"],
    "deny": ["Bash(rm -rf*)", "Bash(git push*)"]
  },
  "hooks": {
    "PostToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "echo hi"}]}],
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo mine"}]}]
  },
  "env": {"A": "1 < 2 & 3"}
}`
	m, err := Merge([]byte(existing))
	if err != nil || !m.Changed {
		t.Fatalf("%+v %v", m, err)
	}
	// Key order is the user's: model, permissions, hooks, env.
	order := []string{`"model"`, `"permissions"`, `"hooks"`, `"env"`}
	last := -1
	for _, k := range order {
		i := strings.Index(m.JSON, "\n  "+k)
		if i <= last {
			t.Errorf("%s out of order:\n%s", k, m.JSON)
		}
		last = i
	}
	for _, keep := range []string{`"Bash(ls*)"`, `"Bash(rm -rf*)"`, `"echo mine"`, `"echo hi"`, `"1 < 2 & 3"`} {
		if !strings.Contains(m.JSON, keep) {
			t.Errorf("lost %s:\n%s", keep, m.JSON)
		}
	}
	if strings.Count(m.JSON, `"Bash(git push*)"`) != 1 {
		t.Error("an existing deny rule must not be duplicated")
	}
	if strings.Count(m.JSON, HookCommand) != 1 {
		t.Error("hook added once")
	}
	again, _ := Merge([]byte(m.JSON))
	if again.Changed || again.JSON != m.JSON {
		t.Errorf("second merge changed things: %v", again.Added)
	}
}

func TestMergeRecognisesTheLegacyNodeHook(t *testing.T) {
	legacy := `{"permissions":{"deny":["Bash(git commit*)","Bash(git push*)","PowerShell(git commit*)","PowerShell(git push*)"]},
	"hooks":{"PreToolUse":[{"matcher":"Bash|PowerShell","hooks":[{"type":"command","shell":"bash","command":"node -e \"...prepare_for_push...\""}]}]}}`
	m, err := Merge([]byte(legacy))
	if err != nil || m.Changed {
		t.Errorf("an installed legacy hook must not be duplicated: %+v %v", m, err)
	}
}

func TestMergeRejectsInvalidJSON(t *testing.T) {
	if _, err := Merge([]byte("{ nope")); err == nil {
		t.Error("expected an error")
	}
}
