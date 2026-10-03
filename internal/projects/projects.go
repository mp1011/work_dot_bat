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
	"strings"
)

// RegistryEnv overrides the registry file location.
const RegistryEnv = "WORK_DOT_BAT_REGISTRY"

type Project struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ToolRoot is the first of the executable's directory and the working directory that holds a
// projects.json or projects.example.json (so `go run` from the checkout works); failing
// that, the executable's directory (or the working directory).
func ToolRoot() string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
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

func samePath(a, b string) bool {
	norm := func(p string) string {
		return strings.ToLower(strings.TrimRight(strings.ReplaceAll(p, `\`, "/"), "/"))
	}
	return norm(a) == norm(b)
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
		seen[name] = true
		projects = append(projects, Project{Name: name, Path: p})
	}
	return projects, errs
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
