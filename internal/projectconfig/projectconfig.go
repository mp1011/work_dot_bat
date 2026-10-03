// Package projectconfig loads <repo>/work_dot_bat.config.json, committed in the target repo
// so CI, other machines and worktrees all see the same commands.
package projectconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const ConfigFile = "work_dot_bat.config.json"

const DefaultDaemonDoc = "agent_docs/instructions/high_trust/do-work-yolo.md"

// CommandFields are the commands prompted for when registering a repo, in prompt order.
var CommandFields = []string{"build", "lightTest", "heavyTest", "install", "devServer"}

// ToolCheckIDs are the tool-owned check ids, in the order they run (mirrors the vendored run-checks.mjs).
var ToolCheckIDs = []string{
	"plan-drift", "monolith", "duplication", "structural-duplication",
	"source-assertions", "skills", "audit",
}

type Command struct {
	Cmd   string            `json:"cmd"`
	Args  []string          `json:"args"`
	Env   map[string]string `json:"env,omitempty"`
	Shell *bool             `json:"shell,omitempty"`
}

// Config is the commands subset written for a newly registered repo.
type Config struct {
	Build                []Command `json:"build,omitempty"`
	LightTest            *Command  `json:"lightTest,omitempty"`
	HeavyTest            *Command  `json:"heavyTest,omitempty"`
	Install              *Command  `json:"install,omitempty"`
	DevServer            *Command  `json:"devServer,omitempty"`
	ProjectStructureDoc  string    `json:"projectStructureDoc,omitempty"`
	ProjectStructureDirs []string  `json:"projectStructureDirs,omitempty"`
}

func cmd(c string, args ...string) *Command { return &Command{Cmd: c, Args: args} }

func NpmDefaults() Config {
	return Config{
		Build: []Command{
			{Cmd: "npm", Args: []string{"run", "lint"}},
			{Cmd: "npx", Args: []string{"tsc", "--noEmit"}},
		},
		LightTest: cmd("npm", "test", "--", "--run"),
		HeavyTest: cmd("npm", "run", "test:e2e"),
		Install:   cmd("npm", "install"),
		DevServer: cmd("npm", "run", "dev"),
	}
}

// ParseCommandLine is a whitespace split; anything needing quoting is edited in the config file.
func ParseCommandLine(line string) *Command {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	return &Command{Cmd: parts[0], Args: append([]string{}, parts[1:]...)}
}

func FormatCommand(c Command) string {
	return strings.Join(append([]string{c.Cmd}, c.Args...), " ")
}

func isCommand(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if s, ok := m["cmd"].(string); !ok || s == "" {
		return false
	}
	args, ok := m["args"].([]any)
	if !ok {
		return false
	}
	for _, a := range args {
		if _, ok := a.(string); !ok {
			return false
		}
	}
	if env, present := m["env"]; present {
		if _, ok := env.(map[string]any); !ok {
			return false
		}
	}
	if sh, present := m["shell"]; present {
		if _, ok := sh.(bool); !ok {
			return false
		}
	}
	return true
}

func allStrings(v any) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, e := range list {
		if _, ok := e.(string); !ok {
			return false
		}
	}
	return true
}

func nonEmptyString(v any) bool { s, ok := v.(string); return ok && s != "" }

// Validate returns human-readable problems; empty means valid. Unknown fields are allowed.
func Validate(config any) []string {
	c, ok := config.(map[string]any)
	if !ok {
		return []string{"config: expected a JSON object"}
	}
	var errs []string
	if v, ok := c["build"]; ok {
		list, isList := v.([]any)
		valid := isList
		for _, e := range list {
			valid = valid && isCommand(e)
		}
		if !valid {
			errs = append(errs, `"build": expected an array of { cmd, args } commands`)
		}
	}
	for _, f := range []string{"lightTest", "heavyTest", "install"} {
		if v, ok := c[f]; ok && !isCommand(v) {
			errs = append(errs, fmt.Sprintf(`"%s": expected a { cmd, args } command`, f))
		}
	}
	if v, ok := c["devServer"]; ok && v != nil && !isCommand(v) {
		errs = append(errs, `"devServer": expected a { cmd, args } command or null`)
	}
	if v, ok := c["checks"]; ok {
		list, isList := v.([]any)
		if !isList {
			errs = append(errs, `"checks": expected an array of check ids`)
		}
		for _, id := range list {
			s, _ := id.(string)
			if !slices.Contains(ToolCheckIDs, s) {
				errs = append(errs, fmt.Sprintf(`"checks": unknown check id "%v" (known: %s)`, id, strings.Join(ToolCheckIDs, ", ")))
			}
		}
	}
	if v, ok := c["extraChecks"]; ok {
		list, isList := v.([]any)
		valid := isList
		for _, e := range list {
			m, isObj := e.(map[string]any)
			valid = valid && isObj && nonEmptyString(m["name"]) && isCommand(m["command"])
		}
		if !valid {
			errs = append(errs, `"extraChecks": expected an array of { name, command: { cmd, args } }`)
		}
	}
	if v, ok := c["worktreeCopyFiles"]; ok && !allStrings(v) {
		errs = append(errs, `"worktreeCopyFiles": expected an array of repo-relative paths`)
	}
	if v, ok := c["agents"]; ok {
		if _, isObj := v.(map[string]any); !isObj {
			errs = append(errs, `"agents": expected an object like { claude: {...}, agy: {...} }`)
		}
	}
	for _, f := range []string{"projectStructureDoc", "planTemplate"} {
		if v, ok := c[f]; ok && !nonEmptyString(v) {
			errs = append(errs, fmt.Sprintf(`"%s": expected a repo-relative path`, f))
		}
	}
	if v, ok := c["instructions"]; ok {
		m, isObj := v.(map[string]any)
		_, daemonIsString := m["daemonDoc"].(string)
		if !isObj || (m["daemonDoc"] != nil && !daemonIsString) {
			errs = append(errs, `"instructions": expected an object like { daemonDoc: "path" }`)
		}
	}
	return errs
}

type Loaded struct {
	Config map[string]any
	Errors []string
	Exists bool
}

// Load: a missing file is not an error ({nil, exists: false}).
func Load(repoRoot string) Loaded {
	file := filepath.Join(repoRoot, ConfigFile)
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Loaded{}
	}
	if err != nil {
		return Loaded{Errors: []string{file + ": " + err.Error()}, Exists: true}
	}
	var config any
	if err := json.Unmarshal(b, &config); err != nil {
		return Loaded{Errors: []string{file + ": " + err.Error()}, Exists: true}
	}
	out := Loaded{Exists: true}
	out.Config, _ = config.(map[string]any)
	for _, e := range Validate(config) {
		out.Errors = append(out.Errors, file+": "+e)
	}
	return out
}

func Write(repoRoot string, config Config) (string, error) {
	file := filepath.Join(repoRoot, ConfigFile)
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	return file, os.WriteFile(file, append(b, '\n'), 0o644)
}

// NewRepoConfig is the config written for a newly registered repo.
func NewRepoConfig(commands Config) Config {
	commands.ProjectStructureDoc = "agent_docs/02_project_structure.md"
	commands.ProjectStructureDirs = []string{"src"}
	return commands
}

// ForProject reads the selected project's config. A worktree's checkout only has the file
// once committed, so treeRoot falls back to projectRoot. Errors name where it looked.
func ForProject(treeRoot, projectRoot string) (map[string]any, error) {
	roots := []string{treeRoot}
	if treeRoot != projectRoot {
		roots = append(roots, projectRoot)
	}
	for _, root := range roots {
		l := Load(root)
		if !l.Exists {
			continue
		}
		if len(l.Errors) > 0 || l.Config == nil {
			return nil, fmt.Errorf("invalid %s:\n  %s", ConfigFile, strings.Join(l.Errors, "\n  "))
		}
		return l.Config, nil
	}
	return nil, fmt.Errorf("no %s found in %s. Register the project through the launcher menu (it writes one), or create it by hand",
		ConfigFile, strings.Join(roots, " or "))
}

// DaemonDoc returns instructions.daemonDoc, or the default when unavailable.
func DaemonDoc(treeRoot, projectRoot string) string {
	c, err := ForProject(treeRoot, projectRoot)
	if err != nil {
		return DefaultDaemonDoc
	}
	if i, ok := c["instructions"].(map[string]any); ok {
		if s, ok := i["daemonDoc"].(string); ok {
			return s
		}
	}
	return DefaultDaemonDoc
}
