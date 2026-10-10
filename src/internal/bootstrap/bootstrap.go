// Package bootstrap prepares a repository to be driven by work_dot_bat: agent docs, instructions,
// skills, the safety hook, the vendored scripts, baselines, the state-folder ignore entry and a
// pre-push hook. Nothing that already exists is overwritten: it is skipped and reported.
package bootstrap

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"work_dot_bat/src/internal/managed"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/safetyhook"
	"work_dot_bat/src/internal/statemigrate"
	"work_dot_bat/src/internal/syncer"
)

const (
	StructureDoc = "agent_docs/02_project_structure.md"
	PlanTemplate = "agent_docs/plan_template.md"
)

// repoConfig is the slice of work_dot_bat.config.json that bootstrap reads.
type repoConfig struct {
	Build               []projectconfig.Command `json:"build"`
	LightTest           *projectconfig.Command  `json:"lightTest"`
	HeavyTest           *projectconfig.Command  `json:"heavyTest"`
	Install             *projectconfig.Command  `json:"install"`
	DevServer           *projectconfig.Command  `json:"devServer"`
	ProjectStructureDoc string                  `json:"projectStructureDoc"`
	PlanTemplate        string                  `json:"planTemplate"`
	Checks              []string                `json:"checks"`
	ChecksSet           bool                    `json:"-"`
}

func toRepoConfig(v any) repoConfig {
	b, _ := json.Marshal(v)
	var c repoConfig
	_ = json.Unmarshal(b, &c)
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) == nil {
		_, c.ChecksSet = raw["checks"]
	}
	return c
}

const none = "(none configured)"

func tick(c projectconfig.Command) string { return "`" + projectconfig.FormatCommand(c) + "`" }

func cmdText(c *projectconfig.Command) string {
	if c == nil {
		return none
	}
	return tick(*c)
}

// Values are the template placeholder values for a project.
func Values(c repoConfig, projectName string) map[string]string {
	build := none
	if len(c.Build) > 0 {
		parts := make([]string, len(c.Build))
		for i, b := range c.Build {
			parts[i] = tick(b)
		}
		build = strings.Join(parts, ", ")
	}
	orDefault := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}
	return map[string]string{
		"project_name":          projectName,
		"project_structure_doc": orDefault(c.ProjectStructureDoc, StructureDoc),
		"plan_template":         orDefault(c.PlanTemplate, PlanTemplate),
		"build_commands":        build,
		"light_test":            cmdText(c.LightTest),
		"heavy_test":            cmdText(c.HeavyTest),
		"install":               cmdText(c.Install),
		"dev_server":            cmdText(c.DevServer),
	}
}

var placeholderRE = regexp.MustCompile(`\{\{([a-z_]+)\}\}`)

// Render fills {{placeholders}}; an unknown one is an error, so a typo in a template cannot ship
// as a literal "{{...}}".
func Render(text string, values map[string]string) (string, error) {
	var err error
	out := placeholderRE.ReplaceAllStringFunc(text, func(m string) string {
		key := m[2 : len(m)-2]
		v, ok := values[key]
		if !ok && err == nil {
			err = fmt.Errorf("Unknown template placeholder {{%s}}", key)
		}
		return v
	})
	return out, err
}

func listFiles(fsys fs.FS) []string {
	var out []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// DetectHookManager names a hook manager that owns .git/hooks (we leave those alone and print
// instructions instead), or "".
func DetectHookManager(root string) string {
	has := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	switch {
	case has(".husky"):
		return "husky"
	case has("lefthook.yml") || has("lefthook.yaml"):
		return "lefthook"
	case has(".pre-commit-config.yaml"):
		return "pre-commit"
	case has("package.json"):
		if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
			var pkg map[string]any
			if json.Unmarshal(b, &pkg) == nil && (pkg["simple-git-hooks"] != nil || pkg["husky"] != nil) {
				return "simple-git-hooks"
			}
		}
	}
	return ""
}

const prePush = "#!/bin/sh\n# Installed by work_dot_bat. Runs this repo's mechanical gate before every push.\nexec scripts/work_dot_bat/run-checks.sh\n"

type Kind string

const (
	KindFile      Kind = "file"
	KindSettings  Kind = "settings"
	KindGitignore Kind = "gitignore"
	KindHook      Kind = "hook"
	KindConfig    Kind = "config"
)

type Status string

const (
	Create Status = "create"
	Merge  Status = "merge"
	Exists Status = "exists"
	OK     Status = "ok"
	Manual Status = "manual"
)

// Action is one thing bootstrap would do.
type Action struct {
	Path    string
	Kind    Kind
	Status  Status
	Content string
	Note    string
}

var defaultChecks = []string{"monolith", "duplication", "structural-duplication"}

// A fresh repo starts every ratchet at zero tolerance.
const (
	monolithBaseline = "{\n  \"maxLines\": 500,\n  \"entries\": {}\n}\n"
	dupBaseline      = "{\n  \"totalDuplicatedBlocks\": 0\n}\n"
)

// Plan computes what bootstrap would do for root.
func Plan(root string, c repoConfig, projectName string, templates fs.FS) ([]Action, error) {
	var actions []Action
	full := func(r string) string { return filepath.Join(root, filepath.FromSlash(r)) }
	exists := func(r string) bool { _, err := os.Stat(full(r)); return err == nil }
	values := Values(c, projectName)

	// 1. templated files. Two live wherever the project's config says (a repo that already has a
	// structure doc or a plan template keeps it), so they are skipped, not duplicated.
	remap := map[string]string{StructureDoc: values["project_structure_doc"], PlanTemplate: values["plan_template"]}
	for _, file := range listFiles(templates) {
		raw, err := fs.ReadFile(templates, file)
		if err != nil {
			return nil, err
		}
		content, err := Render(string(raw), values)
		if err != nil {
			return nil, err
		}
		target := file
		if r, ok := remap[file]; ok {
			target = r
		}
		st := Create
		if exists(target) {
			st = Exists
		}
		actions = append(actions, Action{Path: target, Kind: KindFile, Status: st, Content: content})
	}

	// 2. folders agents expect to exist
	for _, dir := range []string{"agent_docs/deep_dives", "agent_docs/plans", "agent_docs/project_health"} {
		st := Create
		if exists(dir) {
			st = Exists
		}
		actions = append(actions, Action{Path: dir + "/.gitkeep", Kind: KindFile, Status: st})
	}

	// 3. baselines for the enabled ratchets
	enabled := defaultChecks
	if c.ChecksSet {
		enabled = c.Checks
	}
	for _, b := range []struct{ id, file, content string }{
		{"monolith", "agent_docs/monolith_baseline.json", monolithBaseline},
		{"duplication", "agent_docs/duplication_baseline.json", dupBaseline},
		{"structural-duplication", "agent_docs/structural_duplication_baseline.json", dupBaseline},
	} {
		if !slices.Contains(enabled, b.id) {
			continue
		}
		st := Create
		if exists(b.file) {
			st = Exists
		}
		actions = append(actions, Action{Path: b.file, Kind: KindFile, Status: st, Content: b.content})
	}

	// 4. safety settings: merged into an existing file, never overwritten
	const settingsPath = ".claude/settings.json"
	var existing []byte
	invalid := false
	if exists(settingsPath) {
		existing, _ = os.ReadFile(full(settingsPath))
		var probe map[string]any
		if json.Unmarshal(existing, &probe) != nil {
			invalid = true
			actions = append(actions, Action{Path: settingsPath, Kind: KindSettings, Status: Manual,
				Note: "exists but is not valid JSON; merge the safety hook by hand"})
		}
	}
	if !invalid {
		merged, err := safetyhook.Merge(existing)
		if err != nil {
			return nil, err
		}
		st := Create
		switch {
		case !merged.Changed:
			st = OK
		case existing != nil:
			st = Merge
		}
		actions = append(actions, Action{Path: settingsPath, Kind: KindSettings, Status: st, Content: merged.JSON, Note: strings.Join(merged.Added, ", ")})
	}

	// 5. gitignore entry for the state folder
	st := Create
	switch {
	case statemigrate.GitignoreCoversState(root):
		st = OK
	case exists(".gitignore"):
		st = Merge
	}
	actions = append(actions, Action{Path: ".gitignore", Kind: KindGitignore, Status: st})

	// 6. pre-push hook
	manager := DetectHookManager(root)
	alreadyRuns := false
	if manager == "husky" && exists(".husky/pre-push") {
		b, _ := os.ReadFile(full(".husky/pre-push"))
		alreadyRuns = strings.Contains(string(b), "run-checks.sh")
	}
	switch {
	case alreadyRuns:
		actions = append(actions, Action{Path: ".husky/pre-push", Kind: KindHook, Status: OK})
	case manager != "":
		actions = append(actions, Action{Path: ".git/hooks/pre-push", Kind: KindHook, Status: Manual,
			Note: manager + " manages hooks here; make its pre-push hook run scripts/work_dot_bat/run-checks.sh"})
	default:
		st := Create
		if exists(".git/hooks/pre-push") {
			st = Exists
		}
		actions = append(actions, Action{Path: ".git/hooks/pre-push", Kind: KindHook, Status: st, Content: prePush})
	}
	return actions, nil
}

// Apply performs the create/merge actions and returns the ones it did.
func Apply(root string, actions []Action) ([]Action, error) {
	full := func(r string) string { return filepath.Join(root, filepath.FromSlash(r)) }
	write := func(r, content string) error {
		if err := os.MkdirAll(filepath.Dir(full(r)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full(r), []byte(content), 0o644)
	}
	var done []Action
	for _, a := range actions {
		if a.Status != Create && a.Status != Merge {
			continue
		}
		switch a.Kind {
		case KindFile, KindSettings:
			if err := write(a.Path, a.Content); err != nil {
				return done, err
			}
		case KindGitignore:
			prior, _ := os.ReadFile(full(".gitignore"))
			sep := ""
			if len(prior) > 0 && !strings.HasSuffix(string(prior), "\n") {
				sep = "\n"
			}
			text := string(prior) + sep + "\n# work_dot_bat runtime state (claims, session ledger, logs)\n" + repo.StateDirName + "/\n"
			if err := os.WriteFile(full(".gitignore"), []byte(text), 0o644); err != nil {
				return done, err
			}
		case KindHook:
			if err := write(a.Path, a.Content); err != nil {
				return done, err
			}
			_ = os.Chmod(full(a.Path), 0o755) // best effort
		}
		done = append(done, a)
	}
	return done, nil
}

// PrerequisiteNotes are things the repo must provide itself for the vendored checks to run.
// Reported, never fixed: installing packages is the user's call.
func PrerequisiteNotes(root string, c repoConfig) []string {
	enabled := []string{"structural-duplication", "source-assertions"}
	if c.ChecksSet {
		enabled = c.Checks
	}
	if !slices.Contains(enabled, "structural-duplication") && !slices.Contains(enabled, "source-assertions") {
		return nil
	}
	deps := map[string]any{}
	if b, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Dependencies    map[string]any `json:"dependencies"`
			DevDependencies map[string]any `json:"devDependencies"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			for k, v := range pkg.Dependencies {
				deps[k] = v
			}
			for k, v := range pkg.DevDependencies {
				deps[k] = v
			}
		}
	}
	if _, ok := deps["typescript"]; ok {
		return nil
	}
	return []string{"The structural-duplication and source-assertions checks import `typescript`: add it to devDependencies (npm install -D typescript) or remove those ids from `checks` in work_dot_bat.config.json."}
}

// Result is what a full bootstrap did (or, dry, would do).
type Result struct {
	Actions       []Action
	Sync          syncer.Result
	SyncPlan      syncer.Plan
	ConfigCreated bool
	Notes         []string
}

// Options for Run.
type Options struct {
	ProjectName  string
	DryRun       bool
	Vendor       fs.FS // the vendored files, as in a target repo
	Templates    fs.FS // templates/files
	Today        string
	RegistryFile string
}

// Run is the whole flow: config first (never overwritten), then the files, then sync.
func Run(root string, o Options) (Result, error) {
	if !syncer.IsMainCheckout(root) {
		return Result{}, fmt.Errorf("%s is not a main git checkout (no .git directory); bootstrap the main checkout, not a worktree.", root)
	}

	registryFile := o.RegistryFile
	if registryFile == "" {
		registryFile = projects.DefaultRegistryFile()
	}

	projs, _, _ := projects.LoadRegistry(registryFile)
	proj := projects.FindProjectByPath(projs, root)

	var cfg repoConfig
	created := false

	if proj != nil && proj.HasConfig() {
		cfg = toRepoConfig(proj.ToConfigMap())
	} else {
		loaded := projectconfig.Load(root)
		if loaded.Exists {
			if len(loaded.Errors) > 0 {
				return Result{}, fmt.Errorf("Invalid %s:\n  %s", projectconfig.ConfigFile, strings.Join(loaded.Errors, "\n  "))
			}
			cfg = toRepoConfig(loaded.Config)
		} else {
			created = true
			name := o.ProjectName
			if name == "" || name == "this project" {
				name = projects.DefaultProjectName(root)
			}
			freshDefaults := projectconfig.NpmDefaults()
			freshProj := projects.Project{
				Name:                 name,
				Path:                 root,
				Build:                freshDefaults.Build,
				Install:              freshDefaults.Install,
				DevServer:            freshDefaults.DevServer,
				ProjectStructureDoc:  StructureDoc,
				ProjectStructureDirs: []string{"src"},
			}
			if freshDefaults.LightTest != nil {
				freshProj.Tests.Light = [][]string{append([]string{freshDefaults.LightTest.Cmd}, freshDefaults.LightTest.Args...)}
			}
			if freshDefaults.HeavyTest != nil {
				freshProj.Tests.Heavy = [][]string{append([]string{freshDefaults.HeavyTest.Cmd}, freshDefaults.HeavyTest.Args...)}
			}
			cfg = toRepoConfig(freshProj.ToConfigMap())

			if !o.DryRun {
				var updated []projects.Project
				if proj != nil {
					for _, p := range projs {
						if projects.SamePath(p.Path, root) {
							p.Build = freshProj.Build
							p.Install = freshProj.Install
							p.DevServer = freshProj.DevServer
							p.Tests = freshProj.Tests
							p.ProjectStructureDoc = freshProj.ProjectStructureDoc
							p.ProjectStructureDirs = freshProj.ProjectStructureDirs
						}
						updated = append(updated, p)
					}
				} else {
					var err error
					updated, err = projects.AddProject(projs, freshProj)
					if err != nil {
						return Result{}, err
					}
				}
				if err := projects.SaveRegistry(registryFile, updated); err != nil {
					return Result{}, err
				}
			}
		}
	}

	actions, err := Plan(root, cfg, o.ProjectName, o.Templates)
	if err != nil {
		return Result{}, err
	}
	if created {
		status := Create
		if proj != nil {
			status = Merge
		}
		actions = append([]Action{{Path: registryFile, Kind: KindConfig, Status: status}}, actions...)
	}
	if !o.DryRun {
		if _, err := Apply(root, actions); err != nil {
			return Result{}, err
		}
	}

	manifest, err := managed.ReadManifest(o.Vendor)
	if err != nil {
		return Result{}, err
	}
	plan := syncer.PlanSync(root, manifest)
	res, err := syncer.Apply(root, plan, manifest, o.Vendor, syncer.Options{DryRun: o.DryRun, Today: o.Today})
	if err != nil {
		return Result{}, err
	}
	return Result{Actions: actions, Sync: res, SyncPlan: plan, ConfigCreated: created, Notes: PrerequisiteNotes(root, cfg)}, nil
}

// FormatReport is the text `bootstrap` prints.
func FormatReport(r Result, dryRun bool) string {
	var lines []string
	verb := map[Status]string{Create: "Created", Merge: "Updated"}
	if dryRun {
		verb = map[Status]string{Create: "Would create", Merge: "Would update"}
	}
	for _, a := range r.Actions {
		switch a.Status {
		case Create, Merge:
			note := ""
			if a.Note != "" {
				note = " (" + a.Note + ")"
			}
			lines = append(lines, fmt.Sprintf("%s: %s%s", verb[a.Status], a.Path, note))
		case Exists:
			lines = append(lines, "Skipped (already exists): "+a.Path)
		case Manual:
			lines = append(lines, fmt.Sprintf("Manual step: %s — %s", a.Path, a.Note))
		}
	}
	install := "Installed"
	if dryRun {
		install = "Would install"
	}
	for _, i := range r.Sync.Written {
		lines = append(lines, fmt.Sprintf("%s: %s", install, i.File))
	}
	for _, i := range r.Sync.Skipped {
		lines = append(lines, "Skipped (edited locally): "+i.File)
	}
	if len(lines) == 0 {
		lines = append(lines, "Nothing to do: this repo is already bootstrapped.")
	} else if !dryRun {
		lines = append(lines, "Nothing is committed. Review the new files and commit them yourself.")
	}
	for _, n := range r.Notes {
		lines = append(lines, "Note: "+n)
	}
	return strings.Join(lines, "\n")
}
