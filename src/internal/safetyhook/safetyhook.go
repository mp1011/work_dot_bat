// Package safetyhook is the Claude Code safety configuration that stops an agent from committing,
// pushing, or running the interactive tools that can (work, prepare_for_push) on the user's
// behalf. Bootstrap installs it into target repos and this repo's own .claude/settings.json uses
// it. The hook itself is the `work_dot_bat safety-hook` subcommand, not an inline script.
package safetyhook

import (
	"bytes"
	"encoding/json"
	"regexp"
)

var DenyRules = []string{"Bash(git commit*)", "Bash(git push*)", "PowerShell(git commit*)", "PowerShell(git push*)"}

const (
	PushMessage = "Blocked: the prepare-for-push script performs a real git push and must be run by the human directly from their own terminal, not invoked by Claude."
	WorkMessage = "Blocked: work is an interactive human tool that can commit, push, and set auto: true on a plan; it must be run by the human directly from their own terminal, not invoked by Claude."
	GitMessage  = "Blocked: git commit and git push must be run by the human directly from their own terminal, not invoked by Claude."

	// HookCommand is what settings.json runs for every Bash/PowerShell tool call. It reads the
	// tool-call JSON on stdin and exits 2 (with the message on stderr) to block it.
	HookCommand = `bin="${WORK_DOT_BAT_BIN:-work_dot_bat}"; if ! command -v "$bin" >/dev/null 2>&1; then echo "work_dot_bat binary not found: put it on PATH or set WORK_DOT_BAT_BIN." >&2; exit 2; fi; exec "$bin" safety-hook`
	// hookMarker recognises an installed hook; legacyMarker an older inline Node one.
	hookMarker   = "safety-hook"
	legacyMarker = "prepare_for_push"
)

// The entry points: the Node-era .js/.bat and launch forms, and the binary's own subcommands.
// `\b` after `work` keeps the `work_dot_bat` name itself from tripping the rule.
var (
	pushRE = regexp.MustCompile(`(?i)prepare_for_push\.(js|bat)\b|launch\.mjs["']?\s+prepare_for_push\b|work_dot_bat(\.exe)?["']?\s+prepare_for_push\b`)
	workRE = regexp.MustCompile(`(?i)\bwork\.(js|bat)\b|launch\.mjs["']?\s+work\b|work_dot_bat(\.exe)?["']?\s+work\b`)
	gitRE  = regexp.MustCompile(`(?i)\bgit(\.exe)?\s+(commit|push)\b`)
)

// Check decides one hook call. input is the hook JSON from stdin; it returns the exit code (0
// allow, 2 block) and the message for stderr. Anything unparseable is allowed, as before.
func Check(input []byte) (code int, message string) {
	var call struct {
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(input, &call) != nil {
		return 0, ""
	}
	cmd := call.ToolInput.Command
	switch {
	case pushRE.MatchString(cmd):
		return 2, PushMessage
	case workRE.MatchString(cmd):
		return 2, WorkMessage
	case gitRE.MatchString(cmd):
		return 2, GitMessage
	}
	return 0, ""
}

type hookEntry struct {
	Matcher string     `json:"matcher"`
	Hooks   []hookSpec `json:"hooks"`
}

type hookSpec struct {
	Type    string `json:"type"`
	Shell   string `json:"shell"`
	Command string `json:"command"`
}

// obj is a JSON object that remembers its key order, so merging never reshuffles a user's file.
type obj struct {
	keys []string
	vals map[string]json.RawMessage
}

func newObj() *obj { return &obj{vals: map[string]json.RawMessage{}} }

func parseObj(b []byte) (*obj, error) {
	var vals map[string]json.RawMessage
	if err := json.Unmarshal(b, &vals); err != nil {
		return nil, err
	}
	o := &obj{vals: vals}
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		o.keys = append(o.keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (o *obj) set(k string, v json.RawMessage) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func marshalPlain(v any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func (o *obj) raw() json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshalPlain(k))
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

func objOrNew(raw json.RawMessage, ok bool) (*obj, error) {
	if !ok || string(raw) == "null" {
		return newObj(), nil
	}
	return parseObj(raw)
}

func arrOrNew(raw json.RawMessage, ok bool) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if !ok || string(raw) == "null" {
		return arr, nil
	}
	return arr, json.Unmarshal(raw, &arr)
}

func hasHook(entries []json.RawMessage) bool {
	for _, e := range entries {
		var he struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if json.Unmarshal(e, &he) != nil {
			continue
		}
		for _, h := range he.Hooks {
			if regexp.MustCompile(hookMarker + "|" + legacyMarker).MatchString(h.Command) {
				return true
			}
		}
	}
	return false
}

// Merged is the settings after adding the deny rules and the hook.
type Merged struct {
	JSON    string // indented two spaces, trailing newline
	Changed bool
	Added   []string
}

// Merge adds the deny rules and the hook to existing (a settings document, or nil for none).
// Nothing already there is removed or reordered.
func Merge(existing []byte) (Merged, error) {
	var root *obj
	var err error
	if existing == nil {
		root = newObj()
	} else if root, err = parseObj(existing); err != nil {
		return Merged{}, err
	}
	var added []string

	perms, err := objOrNew(root.vals["permissions"], root.vals["permissions"] != nil)
	if err != nil {
		return Merged{}, err
	}
	denyRaw, hasDeny := perms.vals["deny"]
	deny, err := arrOrNew(denyRaw, hasDeny)
	if err != nil {
		return Merged{}, err
	}
	for _, rule := range DenyRules {
		found := false
		for _, d := range deny {
			var s string
			if json.Unmarshal(d, &s) == nil && s == rule {
				found = true
			}
		}
		if !found {
			deny = append(deny, marshalPlain(rule))
			added = append(added, rule)
		}
	}
	denyOut := marshalPlain(deny)
	perms.set("deny", denyOut)
	root.set("permissions", perms.raw())

	hooks, err := objOrNew(root.vals["hooks"], root.vals["hooks"] != nil)
	if err != nil {
		return Merged{}, err
	}
	preRaw, hasPre := hooks.vals["PreToolUse"]
	pre, err := arrOrNew(preRaw, hasPre)
	if err != nil {
		return Merged{}, err
	}
	if !hasHook(pre) {
		pre = append(pre, marshalPlain(hookEntry{Matcher: "Bash|PowerShell", Hooks: []hookSpec{{Type: "command", Shell: "bash", Command: HookCommand}}}))
		added = append(added, "PreToolUse safety hook")
	}
	preOut := marshalPlain(pre)
	hooks.set("PreToolUse", preOut)
	root.set("hooks", hooks.raw())

	var out bytes.Buffer
	if err := json.Indent(&out, root.raw(), "", "  "); err != nil {
		return Merged{}, err
	}
	out.WriteByte('\n')
	return Merged{JSON: out.String(), Changed: len(added) > 0, Added: added}, nil
}
