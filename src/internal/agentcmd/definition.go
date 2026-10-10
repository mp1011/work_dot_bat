package agentcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/agentsession"
)

// CLIFile is the project-root file that adds to or overrides the agent definitions.
const CLIFile = "agent_cli.json"

// Reader names for Session.Reader: the transcript formats the session classifier understands.
const (
	ReaderClaude = "claude" // ~/.claude/projects/**/<sessionId>.jsonl
	ReaderAgy    = "agy"    // Antigravity brain transcript, located via the run's log file
)

// Session says how to find and read an agent's chat session after a run.
type Session struct {
	Reader  string `json:"reader"`  // ReaderClaude or ReaderAgy
	Dir     string `json:"dir"`     // transcript root; empty = the reader's default under the home dir
	LogFile bool   `json:"logFile"` // the agent writes a log file (passed via LogFileArgs) that locates the transcript
}

// Definition is how to invoke one agent, interactively and not, and how to read its sessions.
// Every agent is self-contained: the model and other flags are written into its args, so two
// models of one CLI are two agents. Args templates may use {prompt}, {repoRoot}, {sessionId}
// and {logFile}.
type Definition struct {
	ID      string   `json:"id"`      // ledger name, e.g. "claude-sonnet"
	Key     string   `json:"key"`     // menu key, e.g. "c"
	Aliases []string `json:"aliases"` // other accepted spellings of the agent
	Label   string   `json:"label"`   // human name for menus and logs
	Cmd     string   `json:"cmd"`

	Args            []string `json:"args"`            // always
	SessionIDArgs   []string `json:"sessionIdArgs"`   // when a session id is given
	LogFileArgs     []string `json:"logFileArgs"`     // when a log file is given
	InteractiveArgs []string `json:"interactiveArgs"` // last, interactive runs
	PrintArgs       []string `json:"printArgs"`       // last, one-shot runs

	Session Session `json:"session"`
}

// DefaultDefinitions are the two built-in agents.
func DefaultDefinitions() []Definition {
	claude := func(id, key, label, model string, aliases ...string) Definition {
		return Definition{
			ID: id, Key: key, Aliases: aliases, Label: label, Cmd: "claude",
			Args:            []string{"--model", model, "--permission-mode", "auto"},
			SessionIDArgs:   []string{"--session-id", "{sessionId}"},
			InteractiveArgs: []string{"{prompt}"},
			PrintArgs:       []string{"--print", "{prompt}"},
			Session:         Session{Reader: ReaderClaude},
		}
	}
	return []Definition{
		{
			ID: "agy", Key: "a", Aliases: []string{"agy", "antigravity"}, Label: "Google Antigravity (Gemini 3.8 Flash)", Cmd: "agy",
			Args:            []string{"--add-dir", "{repoRoot}", "--model", "gemini-3.8-flash", "--effort", "medium"},
			LogFileArgs:     []string{"--log-file", "{logFile}"},
			InteractiveArgs: []string{"-i", "{prompt}"},
			PrintArgs:       []string{"--print", "{prompt}"},
			Session:         Session{Reader: ReaderAgy, LogFile: true},
		},
		claude("claude-sonnet", "c", "Claude Code (Sonnet)", "sonnet", "claude-sonnet", "claude", "sonnet"),
		claude("claude-opus", "o", "Claude Code (Opus)", "opus", "claude-opus", "opus"),
	}
}

func overlay(base Definition, o Definition) Definition {
	if o.Key != "" {
		base.Key = o.Key
	}
	if o.Aliases != nil {
		base.Aliases = o.Aliases
	}
	if o.Label != "" {
		base.Label = o.Label
	}
	if o.Cmd != "" {
		base.Cmd = o.Cmd
	}
	if o.Args != nil {
		base.Args = o.Args
	}
	if o.SessionIDArgs != nil {
		base.SessionIDArgs = o.SessionIDArgs
	}
	if o.LogFileArgs != nil {
		base.LogFileArgs = o.LogFileArgs
	}
	if o.InteractiveArgs != nil {
		base.InteractiveArgs = o.InteractiveArgs
	}
	if o.PrintArgs != nil {
		base.PrintArgs = o.PrintArgs
	}
	if o.Session.Reader != "" {
		base.Session.Reader = o.Session.Reader
	}
	if o.Session.Dir != "" {
		base.Session.Dir = o.Session.Dir
	}
	if o.Session.LogFile {
		base.Session.LogFile = true
	}
	return base
}

// LoadDefinitions reads dir/agent_cli.json ({"agents":[Definition, ...]}) over the built-ins:
// an entry with a built-in's id overrides its non-empty fields, any other id adds an agent.
// A missing, invalid, or entirely unusable file yields the built-ins.
func LoadDefinitions(dir string) []Definition {
	defs := DefaultDefinitions()
	b, err := os.ReadFile(filepath.Join(dir, CLIFile))
	if err != nil {
		return defs
	}
	var f struct {
		Agents []Definition `json:"agents"`
	}
	if json.Unmarshal(b, &f) != nil {
		return defs
	}
	for _, o := range f.Agents {
		if o.ID == "" {
			continue
		}
		found := false
		for i := range defs {
			if defs[i].ID == o.ID {
				defs[i], found = overlay(defs[i], o), true
				break
			}
		}
		if !found {
			if o.Cmd == "" || o.Key == "" || (o.Session.Reader != ReaderClaude && o.Session.Reader != ReaderAgy) {
				continue // a new agent needs a command, a menu key, and a known session reader
			}
			defs = append(defs, overlay(Definition{ID: o.ID}, o))
		}
	}
	return defs
}

func (d Definition) label() string {
	if d.Label != "" {
		return d.Label
	}
	return d.ID
}

func (d Definition) matches(agent string) bool {
	a := strings.ToLower(strings.TrimSpace(agent))
	if a == "" {
		return false
	}
	if a == strings.ToLower(d.ID) || a == strings.ToLower(d.Key) {
		return true
	}
	for _, al := range d.Aliases {
		if a == strings.ToLower(al) {
			return true
		}
	}
	return false
}

// Dirs applies the definition's Session.Dir to the transcript roots used by its reader.
func (d Definition) Dirs(base agentsession.Dirs) agentsession.Dirs {
	if d.Session.Dir == "" {
		return base
	}
	if d.Session.Reader == ReaderAgy {
		base.AgyBrain = d.Session.Dir
	} else {
		base.Projects = d.Session.Dir
	}
	return base
}

// Agent resolves an agent name (id, menu key, or alias) to its definition. Unknown names fall
// back to the Claude definition, as the old two-agent code did.
func (s Settings) Agent(agent string) Definition {
	defs := s.definitions()
	for _, d := range defs {
		if d.matches(agent) {
			return d
		}
	}
	for _, d := range defs {
		if d.Session.Reader == ReaderClaude {
			return d
		}
	}
	return defs[0]
}

func (s Settings) definitions() []Definition {
	if len(s.Defs) == 0 {
		return DefaultDefinitions()
	}
	return s.Defs
}

// AgentLabel is the human name of agent.
func (s Settings) AgentLabel(agent string) string { return s.Agent(agent).label() }

func expand(tmpl []string, vars map[string]string) []string {
	out := make([]string, 0, len(tmpl))
	for _, t := range tmpl {
		out = append(out, expandOne(t, vars))
	}
	return out
}

func expandOne(t string, vars map[string]string) string {
	var sb strings.Builder
	for {
		i := strings.IndexByte(t, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(t[i:], '}')
		if j < 0 {
			break
		}
		name := t[i+1 : i+j]
		v, ok := vars[name]
		if !ok {
			sb.WriteString(t[:i+j+1]) // not a variable: keep the braces
		} else {
			sb.WriteString(t[:i])
			sb.WriteString(v)
		}
		t = t[i+j+1:]
	}
	sb.WriteString(t)
	return sb.String()
}
