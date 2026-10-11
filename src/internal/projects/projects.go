// Package projects is the project registry: which repositories the tools know about
// (name + machine-specific path). Per-repo commands live in the repo's own
// work_dot_bat.config.json (see package projectconfig), not here.
package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// RegistryEnv overrides the registry file location.
const RegistryEnv = "WORK_DOT_BAT_REGISTRY"

// CommandList represents one or more commands for a test tier.
// In JSON, it can be expressed as a single command array (e.g. ["go", "test", "./..."])
// or as an array of command arrays (e.g. [["npm", "run", "lint"], ["npx", "tsc", "--noEmit"]]).
type CommandList [][]string

func (c CommandList) MarshalJSON() ([]byte, error) {
	if len(c) == 0 {
		return []byte("[]"), nil
	}
	if len(c) == 1 {
		return json.Marshal([]string(c[0]))
	}
	return json.Marshal([][]string(c))
}

func (c *CommandList) UnmarshalJSON(b []byte) error {
	var multi [][]string
	if err := json.Unmarshal(b, &multi); err == nil {
		*c = multi
		return nil
	}
	var single []string
	if err := json.Unmarshal(b, &single); err == nil {
		if len(single) > 0 {
			*c = [][]string{single}
		} else {
			*c = nil
		}
		return nil
	}
	return errors.New("expected string array or array of command arrays")
}

type TestsConfig struct {
	UltraLight CommandList `json:"ultra_light,omitempty"`
	Light      CommandList `json:"light,omitempty"`
	Heavy      CommandList `json:"heavy,omitempty"`
	UltraHeavy CommandList `json:"ultra_heavy,omitempty"`
}

func (t TestsConfig) IsEmpty() bool {
	return len(t.UltraLight) == 0 && len(t.Light) == 0 && len(t.Heavy) == 0 && len(t.UltraHeavy) == 0
}

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

func (c Command) MarshalJSON() ([]byte, error) {
	type rawCommand Command
	return json.Marshal(rawCommand(c))
}

func (c *Command) UnmarshalJSON(b []byte) error {
	type rawCommand Command
	var raw rawCommand
	if err := json.Unmarshal(b, &raw); err == nil && raw.Cmd != "" {
		*c = Command(raw)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil && len(arr) > 0 {
		*c = Command{Cmd: arr[0], Args: arr[1:]}
		return nil
	}
	return errors.New("expected {cmd, args} object or string array")
}

func FormatCommand(c Command) string {
	return strings.Join(append([]string{c.Cmd}, c.Args...), " ")
}

func ParseCommandLine(line string) *Command {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}
	return &Command{Cmd: parts[0], Args: append([]string{}, parts[1:]...)}
}

type ExtraCheck struct {
	Name    string  `json:"name"`
	Command Command `json:"command"`
}

type Project struct {
	Name                 string         `json:"name"`
	Path                 string         `json:"path"`
	Tests                TestsConfig    `json:"tests,omitempty"`
	Build                []Command      `json:"build,omitempty"`
	Install              *Command       `json:"install,omitempty"`
	DevServer            *Command       `json:"devServer,omitempty"`
	Checks               []string       `json:"checks,omitempty"`
	ExtraChecks          []ExtraCheck   `json:"extraChecks,omitempty"`
	WorktreeCopyFiles    []string       `json:"worktreeCopyFiles,omitempty"`
	ProjectStructureDoc  string         `json:"projectStructureDoc,omitempty"`
	ProjectStructureDirs []string       `json:"projectStructureDirs,omitempty"`
	Agents               map[string]any `json:"agents,omitempty"`
	PlanTemplate         string         `json:"planTemplate,omitempty"`
	Instructions         map[string]any `json:"instructions,omitempty"`
}

type rawProject struct {
	Name                 string          `json:"name"`
	Path                 string          `json:"path"`
	Tests                *TestsConfig    `json:"tests,omitempty"`
	Build                []Command       `json:"build,omitempty"`
	Install              *Command        `json:"install,omitempty"`
	DevServer            *Command        `json:"devServer,omitempty"`
	Checks               *[]string       `json:"checks,omitempty"`
	ExtraChecks          []ExtraCheck    `json:"extraChecks,omitempty"`
	WorktreeCopyFiles    []string        `json:"worktreeCopyFiles,omitempty"`
	ProjectStructureDoc  string          `json:"projectStructureDoc,omitempty"`
	ProjectStructureDirs []string        `json:"projectStructureDirs,omitempty"`
	Agents               map[string]any  `json:"agents,omitempty"`
	PlanTemplate         string          `json:"planTemplate,omitempty"`
	Instructions         map[string]any  `json:"instructions,omitempty"`
}

func (p Project) MarshalJSON() ([]byte, error) {
	raw := rawProject{
		Name:                 p.Name,
		Path:                 p.Path,
		Build:                p.Build,
		Install:              p.Install,
		DevServer:            p.DevServer,
		ExtraChecks:          p.ExtraChecks,
		WorktreeCopyFiles:    p.WorktreeCopyFiles,
		ProjectStructureDoc:  p.ProjectStructureDoc,
		ProjectStructureDirs: p.ProjectStructureDirs,
		Agents:               p.Agents,
		PlanTemplate:         p.PlanTemplate,
		Instructions:         p.Instructions,
	}
	if p.Checks != nil {
		raw.Checks = &p.Checks
	}
	if !p.Tests.IsEmpty() {
		raw.Tests = &p.Tests
	}
	return json.Marshal(raw)
}

func (p *Project) UnmarshalJSON(b []byte) error {
	var raw rawProject
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	p.Name = raw.Name
	p.Path = raw.Path
	if raw.Tests != nil {
		p.Tests = *raw.Tests
	} else {
		p.Tests = TestsConfig{}
	}
	p.Build = raw.Build
	p.Install = raw.Install
	p.DevServer = raw.DevServer
	if raw.Checks != nil {
		p.Checks = *raw.Checks
	}
	p.ExtraChecks = raw.ExtraChecks
	p.WorktreeCopyFiles = raw.WorktreeCopyFiles
	p.ProjectStructureDoc = raw.ProjectStructureDoc
	p.ProjectStructureDirs = raw.ProjectStructureDirs
	p.Agents = raw.Agents
	p.PlanTemplate = raw.PlanTemplate
	p.Instructions = raw.Instructions
	return nil
}

func (p Project) HasConfig() bool {
	return !p.Tests.IsEmpty() ||
		len(p.Build) > 0 ||
		p.Install != nil ||
		p.DevServer != nil ||
		p.Checks != nil ||
		len(p.ExtraChecks) > 0 ||
		len(p.WorktreeCopyFiles) > 0 ||
		p.ProjectStructureDoc != "" ||
		len(p.ProjectStructureDirs) > 0 ||
		len(p.Agents) > 0 ||
		p.PlanTemplate != "" ||
		len(p.Instructions) > 0
}

func (p Project) ToConfigMap() map[string]any {
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	delete(m, "name")
	delete(m, "path")
	if m == nil {
		m = make(map[string]any)
	}
	if _, has := m["lightTest"]; !has && len(p.Tests.Light) > 0 && len(p.Tests.Light[0]) > 0 {
		var args []any
		for _, a := range p.Tests.Light[0][1:] {
			args = append(args, a)
		}
		m["lightTest"] = map[string]any{
			"cmd":  p.Tests.Light[0][0],
			"args": args,
		}
	}
	if _, has := m["heavyTest"]; !has && len(p.Tests.Heavy) > 0 && len(p.Tests.Heavy[0]) > 0 {
		var args []any
		for _, a := range p.Tests.Heavy[0][1:] {
			args = append(args, a)
		}
		m["heavyTest"] = map[string]any{
			"cmd":  p.Tests.Heavy[0][0],
			"args": args,
		}
	}
	return m
}

// ToolRoot is the first of the executable's directory (or its parent when inside dist/.tools/bin)
// and the working directory that holds .tools and projects.json or projects.example.json;
// failing that, the executable's directory (or the working directory).
func ToolRoot() string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		parent := filepath.Dir(exeDir)
		if _, err := os.Stat(filepath.Join(parent, ".tools")); err == nil {
			dirs = append(dirs, parent)
		}
		if filepath.Base(exeDir) != "dist" {
			dirs = append(dirs, exeDir)
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, ".tools")); err == nil {
			for _, f := range []string{"projects.json", "projects.example.json"} {
				if _, err := os.Stat(filepath.Join(d, f)); err == nil {
					return d
				}
			}
		}
	}
	for _, d := range dirs {
		for _, f := range []string{"projects.json", "projects.example.json"} {
			if _, err := os.Stat(filepath.Join(d, f)); err == nil {
				return d
			}
		}
	}
	if len(dirs) > 0 {
		return dirs[0]
	}
	return "."
}

// DefaultRegistryFile returns $WORK_DOT_BAT_REGISTRY, else projects.json in the tool root.
func DefaultRegistryFile() string {
	if v := os.Getenv(RegistryEnv); v != "" {
		return v
	}
	return filepath.Join(ToolRoot(), "projects.json")
}

var projectArg = regexp.MustCompile(`^project=(.*)$`)

// ParseProjectArg splits `project=<name>` out of an argument list. The last occurrence wins.
// The returned name is nil when absent.
func ParseProjectArg(argv []string) (name *string, rest []string) {
	for _, arg := range argv {
		if m := projectArg.FindStringSubmatch(arg); m != nil {
			v := m[1]
			name = &v
		} else {
			rest = append(rest, arg)
		}
	}
	return name, rest
}

// IsValidProjectName: names are typed after `project=`, so they must survive a shell and a split on '='.
func IsValidProjectName(name string) bool {
	return name != "" && !strings.ContainsAny(name, " \t\r\n\v\f=")
}

// DefaultProjectName is the last path segment of a repo path.
func DefaultProjectName(repoPath string) string {
	trimmed := strings.TrimRight(repoPath, `\/`)
	i := strings.LastIndexAny(trimmed, `\/`)
	return trimmed[i+1:]
}

func SamePath(a, b string) bool {
	norm := func(p string) string {
		return strings.ToLower(strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/"))
	}
	return norm(a) == norm(b)
}

func samePath(a, b string) bool {
	return SamePath(a, b)
}

// FindProjectByPath finds a project matching the given directory path.
func FindProjectByPath(projects []Project, path string) *Project {
	for i := range projects {
		if samePath(projects[i].Path, path) {
			return &projects[i]
		}
	}
	return nil
}

// ValidateRegistry returns the valid projects and one error per dropped entry, each naming
// the entry's position/name and the offending field.
func ValidateRegistry(data any) (projects []Project, errs []string) {
	obj, ok := data.(map[string]any)
	list, isList := obj["projects"].([]any)
	if !ok || !isList {
		return nil, []string{`registry: expected an object with a "projects" array`}
	}
	seen := map[string]bool{}
	for i, raw := range list {
		entry, isObj := raw.(map[string]any)
		name, _ := entry["name"].(string)
		label := fmt.Sprintf("projects[%d]", i)
		if isObj && name != "" {
			label = fmt.Sprintf("project %q", name)
		}
		if !isObj {
			errs = append(errs, label+": expected an object")
			continue
		}
		if !IsValidProjectName(name) {
			errs = append(errs, label+`: "name" must be a non-empty string without whitespace or "="`)
			continue
		}
		p, _ := entry["path"].(string)
		if strings.TrimSpace(p) == "" {
			errs = append(errs, label+`: "path" must be a non-empty string`)
			continue
		}
		if seen[name] {
			errs = append(errs, label+`: duplicate "name"`)
			continue
		}

		var testsConfig TestsConfig
		if rawTests, hasTests := entry["tests"]; hasTests && rawTests != nil {
			testsObj, isTestsObj := rawTests.(map[string]any)
			if !isTestsObj {
				errs = append(errs, label+`: "tests" must be an object`)
				continue
			}

			var testErrs []string
			knownTiers := map[string]*CommandList{
				"ultra_light": &testsConfig.UltraLight,
				"light":       &testsConfig.Light,
				"heavy":       &testsConfig.Heavy,
				"ultra_heavy": &testsConfig.UltraHeavy,
			}

			tierKeys := []string{"ultra_light", "light", "heavy", "ultra_heavy"}
			for _, key := range tierKeys {
				if rawLayer, exists := testsObj[key]; exists && rawLayer != nil {
					arr, isArr := rawLayer.([]any)
					if !isArr {
						testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" must be an array of strings`, key))
						continue
					}
					if len(arr) == 0 {
						testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" command name must not be empty`, key))
						continue
					}

					var cmdList CommandList
					if _, isNested := arr[0].([]any); isNested {
						nestedErr := false
						for _, item := range arr {
							subArr, ok := item.([]any)
							if !ok {
								testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" must be an array of strings`, key))
								nestedErr = true
								break
							}
							if len(subArr) == 0 {
								testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" command name must not be empty`, key))
								nestedErr = true
								break
							}
							var cmd []string
							for _, elem := range subArr {
								s, isStr := elem.(string)
								if !isStr {
									testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" must be an array of strings`, key))
									nestedErr = true
									break
								}
								cmd = append(cmd, s)
							}
							if nestedErr {
								break
							}
							if strings.TrimSpace(cmd[0]) == "" {
								testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" command name must not be empty`, key))
								nestedErr = true
								break
							}
							cmdList = append(cmdList, cmd)
						}
						if nestedErr {
							continue
						}
					} else {
						var cmd []string
						badElem := false
						for _, elem := range arr {
							s, isStr := elem.(string)
							if !isStr {
								testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" must be an array of strings`, key))
								badElem = true
								break
							}
							cmd = append(cmd, s)
						}
						if badElem {
							continue
						}
						if strings.TrimSpace(cmd[0]) == "" {
							testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s" command name must not be empty`, key))
							continue
						}
						cmdList = CommandList{cmd}
					}
					*(knownTiers[key]) = cmdList
				}
			}

			var unknownKeys []string
			for k := range testsObj {
				if _, ok := knownTiers[k]; !ok {
					unknownKeys = append(unknownKeys, k)
				}
			}
			sort.Strings(unknownKeys)
			for _, k := range unknownKeys {
				testErrs = append(testErrs, label+fmt.Sprintf(`: "tests.%s": unknown test layer`, k))
			}

			if len(testErrs) > 0 {
				errs = append(errs, testErrs...)
				continue
			}
		}

		var configErrs []string

		// build
		if rawBuild, exists := entry["build"]; exists && rawBuild != nil {
			arr, ok := rawBuild.([]any)
			valid := ok
			if ok {
				for _, item := range arr {
					if !isCommandObj(item) {
						valid = false
						break
					}
				}
			}
			if !valid {
				configErrs = append(configErrs, label+`: "build" must be an array of commands`)
			}
		}

		// install
		if rawInstall, exists := entry["install"]; exists && rawInstall != nil {
			if !isCommandObj(rawInstall) {
				configErrs = append(configErrs, label+`: "install" must be a command`)
			}
		}

		// devServer
		if rawDevServer, exists := entry["devServer"]; exists && rawDevServer != nil {
			if !isCommandObj(rawDevServer) {
				configErrs = append(configErrs, label+`: "devServer" must be a command or null`)
			}
		}

		// checks
		if rawChecks, exists := entry["checks"]; exists && rawChecks != nil {
			arr, ok := rawChecks.([]any)
			if !ok {
				configErrs = append(configErrs, label+`: "checks" must be an array of check ids`)
			} else {
				for _, item := range arr {
					s, isStr := item.(string)
					if !isStr || !slices.Contains(ToolCheckIDs, s) {
						configErrs = append(configErrs, label+fmt.Sprintf(`: "checks": unknown check id %q`, item))
					}
				}
			}
		}

		// extraChecks
		if rawExtra, exists := entry["extraChecks"]; exists && rawExtra != nil {
			arr, ok := rawExtra.([]any)
			valid := ok
			if ok {
				for _, item := range arr {
					m, isMap := item.(map[string]any)
					if !isMap {
						valid = false
						break
					}
					n, isStr := m["name"].(string)
					if !isStr || strings.TrimSpace(n) == "" || !isCommandObj(m["command"]) {
						valid = false
						break
					}
				}
			}
			if !valid {
				configErrs = append(configErrs, label+`: "extraChecks" must be an array of { name, command }`)
			}
		}

		// worktreeCopyFiles
		if rawFiles, exists := entry["worktreeCopyFiles"]; exists && rawFiles != nil {
			if !isStringList(rawFiles) {
				configErrs = append(configErrs, label+`: "worktreeCopyFiles" must be an array of strings`)
			}
		}

		// projectStructureDoc
		if rawDoc, exists := entry["projectStructureDoc"]; exists && rawDoc != nil {
			s, ok := rawDoc.(string)
			if !ok || strings.TrimSpace(s) == "" {
				configErrs = append(configErrs, label+`: "projectStructureDoc" must be a non-empty string`)
			}
		}

		// projectStructureDirs
		if rawDirs, exists := entry["projectStructureDirs"]; exists && rawDirs != nil {
			if !isStringList(rawDirs) {
				configErrs = append(configErrs, label+`: "projectStructureDirs" must be an array of strings`)
			}
		}

		// agents
		if rawAgents, exists := entry["agents"]; exists && rawAgents != nil {
			if _, ok := rawAgents.(map[string]any); !ok {
				configErrs = append(configErrs, label+`: "agents" must be an object`)
			}
		}

		// planTemplate
		if rawTmpl, exists := entry["planTemplate"]; exists && rawTmpl != nil {
			s, ok := rawTmpl.(string)
			if !ok || strings.TrimSpace(s) == "" {
				configErrs = append(configErrs, label+`: "planTemplate" must be a non-empty string`)
			}
		}

		// instructions
		if rawInstr, exists := entry["instructions"]; exists && rawInstr != nil {
			if _, ok := rawInstr.(map[string]any); !ok {
				configErrs = append(configErrs, label+`: "instructions" must be an object`)
			}
		}

		if len(configErrs) > 0 {
			errs = append(errs, configErrs...)
			continue
		}

		var proj Project
		b, _ := json.Marshal(entry)
		_ = json.Unmarshal(b, &proj)
		proj.Name = name
		proj.Path = p
		proj.Tests = testsConfig
		seen[name] = true
		projects = append(projects, proj)
	}
	return projects, errs
}

func isCommandObj(v any) bool {
	if m, ok := v.(map[string]any); ok {
		s, sok := m["cmd"].(string)
		if !sok || strings.TrimSpace(s) == "" {
			return false
		}
		args, aok := m["args"].([]any)
		if !aok {
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
	if arr, ok := v.([]any); ok && len(arr) > 0 {
		for _, item := range arr {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return strings.TrimSpace(arr[0].(string)) != ""
	}
	return false
}

func isStringList(v any) bool {
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

// LoadRegistry: a missing file is an empty registry. Unparseable JSON returns an error naming the file.
func LoadRegistry(file string) (projects []Project, errs []string, err error) {
	b, readErr := os.ReadFile(file)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, nil, nil
	}
	if readErr != nil {
		return nil, nil, fmt.Errorf("cannot read project registry %s: %w", file, readErr)
	}
	var data any
	if jsonErr := json.Unmarshal(b, &data); jsonErr != nil {
		return nil, nil, fmt.Errorf("cannot parse project registry %s: %w", file, jsonErr)
	}
	projects, errs = ValidateRegistry(data)
	return projects, errs, nil
}

func SaveRegistry(file string, projects []Project) error {
	if projects == nil {
		projects = []Project{}
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(map[string]any{"projects": projects}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

func FindProject(projects []Project, name string) *Project {
	for i := range projects {
		if projects[i].Name == name {
			return &projects[i]
		}
	}
	return nil
}

// KnownNames lists registered names for error messages.
func KnownNames(projects []Project) string {
	if len(projects) == 0 {
		return "(none registered)"
	}
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return strings.Join(names, ", ")
}

// ResolveProject finds a project by name, or returns an error naming the registered projects.
func ResolveProject(projects []Project, name string) (Project, error) {
	if p := FindProject(projects, name); p != nil {
		return *p, nil
	}
	return Project{}, fmt.Errorf("unknown project %q. Known projects: %s", name, KnownNames(projects))
}

// AddProject returns a new slice; errors if the name or path is already registered.
func AddProject(projects []Project, p Project) ([]Project, error) {
	if !IsValidProjectName(p.Name) {
		return nil, fmt.Errorf(`invalid project name %q: use a non-empty name without whitespace or "="`, p.Name)
	}
	if FindProject(projects, p.Name) != nil {
		return nil, fmt.Errorf("a project named %q is already registered", p.Name)
	}
	for _, q := range projects {
		if samePath(q.Path, p.Path) {
			return nil, fmt.Errorf("%s is already registered as %q", p.Path, q.Name)
		}
	}
	out := append([]Project{}, projects...)
	return append(out, p), nil
}

// CheckRepoRoot: a repo root has a .git entry (a directory for a main checkout, a file for a worktree).
func CheckRepoRoot(repoPath string) (ok bool, reason string) {
	if _, err := os.Stat(repoPath); err != nil {
		return false, repoPath + " does not exist"
	}
	if _, err := os.Lstat(filepath.Join(repoPath, ".git")); err != nil {
		return false, repoPath + " is not the root of a git repository (no .git)"
	}
	return true, ""
}

func FormatMenu(projects []Project) string {
	width := 0
	for _, p := range projects {
		if len(p.Name) > width {
			width = len(p.Name)
		}
	}
	lines := []string{"Select a project:"}
	for i, p := range projects {
		lines = append(lines, fmt.Sprintf("  %d) %-*s   %s", i+1, width, p.Name, p.Path))
	}
	lines = append(lines, "  p) Paste a repository path", "  q) Quit")
	return strings.Join(lines, "\n")
}

type ChoiceKind int

const (
	ChoiceInvalid ChoiceKind = iota
	ChoiceProject
	ChoicePaste
	ChoiceQuit
)

type Choice struct {
	Kind    ChoiceKind
	Project Project
}

var digits = regexp.MustCompile(`^\d+$`)

func ParseMenuChoice(input string, projects []Project) Choice {
	text := strings.ToLower(strings.TrimSpace(input))
	switch text {
	case "p":
		return Choice{Kind: ChoicePaste}
	case "q":
		return Choice{Kind: ChoiceQuit}
	}
	if digits.MatchString(text) {
		var n int
		if _, err := fmt.Sscan(text, &n); err == nil && n >= 1 && n <= len(projects) {
			return Choice{Kind: ChoiceProject, Project: projects[n-1]}
		}
	}
	return Choice{Kind: ChoiceInvalid}
}
