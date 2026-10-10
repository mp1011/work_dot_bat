package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"work_dot_bat/internal/planclaims"
	"work_dot_bat/internal/planstatus"
	"work_dot_bat/internal/proc"
	"work_dot_bat/internal/projectconfig"
	"work_dot_bat/internal/projects"
	"work_dot_bat/internal/repo"
	"work_dot_bat/internal/testrunner"
)

// Command is a configured command (build, test, extra check).
type Command struct {
	Cmd   string            `json:"cmd"`
	Args  []string          `json:"args"`
	Env   map[string]string `json:"env,omitempty"`
	Shell *bool             `json:"shell,omitempty"`
}

type ExtraCheck struct {
	Name    string  `json:"name"`
	Command Command `json:"command"`
}

// Config is the part of work_dot_bat.config.json that run-checks reads.
type Config struct {
	Checks      []string              `json:"checks"`
	ChecksSet   bool                  `json:"-"`
	ExtraChecks []ExtraCheck          `json:"extraChecks"`
	Build       []Command             `json:"build"`
	LightTest   *Command              `json:"lightTest"`
	HeavyTest   *Command              `json:"heavyTest"`
	Tests       *projects.TestsConfig `json:"-"`
}

// LoadConfig reads <root>/work_dot_bat.config.json.
func LoadConfig(root string) (Config, error) {
	file := filepath.Join(root, projectconfig.ConfigFile)
	b, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("No %s in %s", projectconfig.ConfigFile, root)
		}
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("Cannot parse %s: %v", file, err)
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(b, &raw) == nil {
		_, c.ChecksSet = raw["checks"]
	}
	return c, nil
}

// Check is one step of the gate. Tool-owned checks run in-process (Native); the rest are commands.
type Check struct {
	Name  string
	Phase string // "", "unit" or "e2e"
	// Native, if set, runs the check in-process in cwd and returns its exit code.
	Native func(ctx context.Context, cwd string) int
	Cmd    string
	Args   []string
	Shell  bool
	Env    map[string]string
}

func FormatCommand(c Command) string { return strings.Join(append([]string{c.Cmd}, c.Args...), " ") }

// NeedsShell honors an explicit `shell`. Node needed one for the npm/npx shims on Windows; Go
// resolves npm.cmd itself, so only an explicit opt-in is kept.
func NeedsShell(c Command) bool { return c.Shell != nil && *c.Shell }

// Env supplies what the in-process checks need.
type Env struct {
	Out, Err     io.Writer
	Git          GitFunc
	Now          func() time.Time
	// Runner runs a command check; nil uses proc.Run.
	Run          func(ctx context.Context, c Check, cwd string) int
	RegistryFile string
	Projects     []projects.Project
}

func (e Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e Env) git() GitFunc {
	if e.Git != nil {
		return e.Git
	}
	return RealGit
}

type toolCheck struct {
	id, name string
	ratchet  bool
}

var toolChecks = []toolCheck{
	{"plan-drift", "Plan Drift Check", false},
	{"monolith", "Monolith Check", true},
	{"duplication", "Duplication Check", true},
	{"structural-duplication", "Structural Duplication Check", true},
	{"source-assertions", "Source Assertion Check", false},
	{"skills", "Skill Tree Integrity Check", false},
	{"audit", "npm audit Gate", false},
}

// The checks that analyze TypeScript syntax trees run the target repo's own synced copy under
// Node, using the `typescript` package it is already required to have (bootstrap says so). Every
// other check is native.
var nodeCheckScripts = map[string]string{
	"structural-duplication": "check-structural-duplication",
	"source-assertions":      "check-source-assertions",
}

func (e Env) native(id string, noTighten bool) func(context.Context, string) int {
	switch id {
	case "plan-drift":
		return func(_ context.Context, cwd string) int {
			return planstatus.CLI{Root: cwd, Stdout: e.Out, Stderr: e.Err, Collect: planstatus.GitCollectWorktreeStatuses,
				Claims: planclaims.DefaultEnv().ListTaskClaims}.Run([]string{"check"})
		}
	case "monolith":
		return func(_ context.Context, cwd string) int {
			args := []string{"check"}
			if noTighten {
				args = append(args, "--no-tighten")
			}
			return MonolithMain(cwd, args, e.Out, e.Err, e.now())
		}
	case "duplication":
		return func(_ context.Context, cwd string) int {
			args := []string{"check"}
			if noTighten {
				args = append(args, "--no-tighten")
			}
			return DuplicationMain(cwd, args, e.Out, e.Err, e.git())
		}
	case "skills":
		return func(_ context.Context, cwd string) int { return SkillsMain(cwd, []string{"check"}, e.Out, e.Err) }
	case "audit":
		return func(_ context.Context, cwd string) int { return AuditMain(cwd, []string{"check"}, e.Out, e.Err) }
	}
	script := nodeCheckScripts[id]
	return func(ctx context.Context, cwd string) int {
		path := filepath.Join(cwd, "scripts", "work_dot_bat", "typescript", script+".mjs")
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(e.Err, "%s not found in %s — run `work_dot_bat sync` (this check runs under Node).\n", script, cwd)
			return 1
		}
		args := []string{path, "check"}
		if noTighten && id == "structural-duplication" {
			args = append(args, "--no-tighten")
		}
		return e.runCommand(ctx, Check{Cmd: "node", Args: args}, cwd)
	}
}

func (e Env) runCommand(ctx context.Context, c Check, cwd string) int {
	if e.Run != nil {
		return e.Run(ctx, c, cwd)
	}
	name, args := c.Cmd, c.Args
	if c.Shell {
		line := strings.Join(append([]string{c.Cmd}, c.Args...), " ")
		if runtime.GOOS == "windows" {
			name, args = "cmd", []string{"/c", line}
		} else {
			name, args = "sh", []string{"-c", line}
		}
	}
	code, err := proc.Run(ctx, name, args, proc.Options{Dir: cwd, Env: c.Env})
	if err != nil {
		return 1
	}
	return code
}

func fromCommand(c Command, name, phase string) Check {
	return Check{Name: name, Phase: phase, Cmd: c.Cmd, Args: c.Args, Shell: NeedsShell(c), Env: c.Env}
}

// BuildCheckList is the ordered gate for a config: tool-owned checks, the repo's extraChecks, its
// build commands, then test layer commands (or legacy light and heavy tests). noTighten is passed
// to the ratchet checks (CI must not rewrite baselines).
func (e Env) BuildCheckList(c Config, noTighten bool, targetLayers ...string) []Check {
	enabled := projectconfig.ToolCheckIDs
	if c.ChecksSet {
		enabled = c.Checks
	}
	var checks []Check
	for _, t := range toolChecks {
		if contains(enabled, t.id) {
			checks = append(checks, Check{Name: t.name, Native: e.native(t.id, noTighten && t.ratchet)})
		}
	}
	for _, x := range c.ExtraChecks {
		checks = append(checks, fromCommand(x.Command, x.Name, ""))
	}
	for _, b := range c.Build {
		checks = append(checks, fromCommand(b, "Build: "+FormatCommand(b), ""))
	}

	tiers := testrunner.AllLayers
	if len(targetLayers) == 1 && targetLayers[0] != "" && targetLayers[0] != testrunner.LayerAll {
		if t, ok := testrunner.TiersToRun(targetLayers[0]); ok {
			tiers = t
		} else {
			tiers = targetLayers
		}
	} else if len(targetLayers) > 1 {
		tiers = targetLayers
	}

	if c.Tests != nil && !c.Tests.IsEmpty() {
		for _, tier := range tiers {
			cmds, ok := testrunner.CommandsForLayer(projects.Project{Tests: *c.Tests}, tier)
			if !ok {
				continue
			}
			for _, cmd := range cmds {
				if len(cmd) == 0 {
					continue
				}
				checks = append(checks, Check{
					Name:  fmt.Sprintf("[%s] %s", tier, strings.Join(cmd, " ")),
					Phase: "test",
					Cmd:   cmd[0],
					Args:  cmd[1:],
				})
			}
		}
	} else {
		hasLight := contains(tiers, testrunner.LayerLight) || len(targetLayers) == 0
		hasHeavy := contains(tiers, testrunner.LayerHeavy) || contains(tiers, testrunner.LayerUltraHeavy) || len(targetLayers) == 0
		if hasLight && c.LightTest != nil {
			checks = append(checks, fromCommand(*c.LightTest, "Light tests: "+FormatCommand(*c.LightTest), "unit"))
		}
		if hasHeavy && c.HeavyTest != nil {
			checks = append(checks, fromCommand(*c.HeavyTest, "Heavy tests: "+FormatCommand(*c.HeavyTest), "e2e"))
		}
	}
	return checks
}

// RunChecks runs every check, returning the failing and skipped names. Mechanical checks all run
// even if one fails, so every failure shows in one pass; the two test suites are skipped once
// anything before them has failed, rather than running against known-broken ground. Stdin is
// closed for every check: a child that can read it can block waiting for a human.
func (e Env) RunChecks(ctx context.Context, checks []Check, cwd string) (failures, skipped []string) {
	log := func(format string, a ...any) { fmt.Fprintf(e.Out, format+"\n", a...) }
	for _, c := range checks {
		if (c.Phase == "unit" || c.Phase == "e2e" || c.Phase == "test") && len(failures) > 0 {
			log("\n--- %s ---", c.Name)
			log("⏭️  Skipped — %d earlier check(s) failed: %s", len(failures), strings.Join(failures, ", "))
			skipped = append(skipped, c.Name)
			continue
		}
		log("\n--- %s ---", c.Name)
		var code int
		if c.Native != nil {
			code = c.Native(ctx, cwd)
		} else {
			code = e.runCommand(ctx, c, cwd)
		}
		if code != 0 {
			log("❌ %s failed.", c.Name)
			failures = append(failures, c.Name)
		} else {
			log("✅ %s passed.", c.Name)
		}
	}
	log("\n===================================================")
	if len(failures) == 0 {
		log("✅ All automated tests passed!")
	} else {
		log("❌ %d check(s) failed:", len(failures))
		for _, f := range failures {
			log("   - %s", f)
		}
		if len(skipped) > 0 {
			log("⏭️  %d check(s) skipped (an earlier check failed):", len(skipped))
			for _, s := range skipped {
				log("   - %s", s)
			}
		}
	}
	log("===================================================\n")
	return failures, skipped
}

// Options are run-checks' flags.
type Options struct {
	NoTighten, SkipTests bool
	Cwd                  string
}

func ParseArgs(argv []string) (Options, error) {
	var o Options
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--no-tighten":
			o.NoTighten = true
		case "--skip-tests":
			o.SkipTests = true
		case "--cwd":
			i++
			if i < len(argv) {
				o.Cwd = argv[i]
			}
		default:
			return o, fmt.Errorf("Unknown argument: %s", argv[i])
		}
	}
	return o, nil
}

// RunChecksMain is `run-checks [--no-tighten] [--skip-tests] [--cwd <dir>]`: 0 all passed, 1 a check
// failed, 2 bad usage or config.
func (e Env) RunChecksMain(ctx context.Context, argv []string) int {
	o, err := ParseArgs(argv)
	if err != nil {
		fmt.Fprintf(e.Err, "%s\nUsage: run-checks [--no-tighten] [--skip-tests] [--cwd <dir>]\n", err)
		return 2
	}
	root := o.Cwd
	if root == "" {
		if root, err = RepoRoot(); err != nil {
			fmt.Fprintln(e.Err, "run-checks: not inside a git repository")
			return 2
		}
	}
	cfg, err := LoadConfig(root)
	if err != nil {
		fmt.Fprintln(e.Err, err.Error())
		return 2
	}
	checks := e.BuildCheckList(cfg, o.NoTighten)
	if o.SkipTests {
		var kept []Check
		for _, c := range checks {
			if c.Phase == "" {
				kept = append(kept, c)
			}
		}
		checks = kept
	}
	if failures, _ := e.RunChecks(ctx, checks, root); len(failures) > 0 {
		return 1
	}
	return 0
}

// StructureMain is `update-project-structure [--check]`.
func StructureMain(root string, argv []string, out io.Writer, git GitFunc) int {
	check := contains(argv, "--check")
	cfg := ReadStructureConfig(root)
	file := filepath.Join(root, filepath.FromSlash(cfg.Doc))
	b, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(out, err.Error())
		return 1
	}
	docText := string(b)
	disk, err := ListGitFiles(root, cfg.Dirs, git)
	if err != nil {
		fmt.Fprintln(out, err.Error())
		return 1
	}
	newText, added, dropped, err := RegenerateDoc(docText, disk)
	if err != nil {
		fmt.Fprintln(out, err.Error())
		return 1
	}
	changed := newText != docText
	unsummarized := FindUnsummarizedFiles(newText)
	base := filepath.Base(file)

	if check {
		if !changed && len(unsummarized) == 0 {
			fmt.Fprintf(out, "%s is in sync.\n", base)
			return 0
		}
		if len(added) > 0 {
			fmt.Fprintf(out, "New files needing a summary (%d):\n", len(added))
			for _, f := range added {
				fmt.Fprintf(out, "  + %s\n", f)
			}
		}
		if len(dropped) > 0 {
			fmt.Fprintf(out, "Bullets for deleted files (%d):\n", len(dropped))
			for _, d := range dropped {
				fmt.Fprintf(out, "  - %s\n", d.Path)
			}
		}
		if len(unsummarized) > 0 {
			fmt.Fprintf(out, "Files still marked `TODO: summarize` (%d):\n", len(unsummarized))
			for _, f := range unsummarized {
				fmt.Fprintf(out, "  ? %s\n", f)
			}
		}
		if changed {
			fmt.Fprintln(out, "\nRun `work_dot_bat update-project-structure` to apply.")
		}
		return 1
	}

	if changed {
		if err := os.WriteFile(file, []byte(newText), 0o644); err != nil {
			fmt.Fprintln(out, err.Error())
			return 1
		}
	}
	fmt.Fprintf(out, "Updated %s: +%d new, -%d removed.\n", base, len(added), len(dropped))
	if len(added) > 0 {
		fmt.Fprintln(out, "Fill in the TODO: summarize placeholders for:")
		for _, f := range added {
			fmt.Fprintf(out, "  + %s\n", f)
		}
	}
	if len(unsummarized) > 0 {
		fmt.Fprintf(out, "\n%d file(s) still marked `TODO: summarize` — write a one-line summary for each:\n", len(unsummarized))
		for _, f := range unsummarized {
			fmt.Fprintf(out, "  ? %s\n", f)
		}
		return 1
	}
	return 0
}

// Dispatch runs one of the repo-check subcommands against the repo containing the current
// directory; ok is false for other names.
func (e Env) Dispatch(ctx context.Context, name string, args []string) (code int, ok bool) {
	switch name {
	case "run-checks":
		return e.RunChecksMain(ctx, args), true
	case "check-monolith", "check-duplication", "check-skills", "check-audit", "update-project-structure":
	default:
		return 0, false
	}
	root, err := RepoRoot()
	if err != nil {
		fmt.Fprintf(e.Err, "%s: not inside a git repository\n", name)
		return 1, true
	}
	switch name {
	case "check-monolith":
		return MonolithMain(root, args, e.Out, e.Err, e.now()), true
	case "check-duplication":
		return DuplicationMain(root, args, e.Out, e.Err, e.git()), true
	case "check-skills":
		return SkillsMain(root, args, e.Out, e.Err), true
	case "check-audit":
		return AuditMain(root, args, e.Out, e.Err), true
	}
	return StructureMain(root, args, e.Out, e.git()), true
}

// RunAutomatedTests runs the project's whole gate in cwd (a worktree is checked in the worktree,
// never silently against the main checkout) and returns the failing check names. The config comes
// from cwd, falling back to projectRoot's copy for a worktree that has not committed it yet.
// If tests are configured in the project registry (projects.json), those layer commands are executed,
// falling back to legacy work_dot_bat.config.json test commands when not defined.
func (e Env) RunAutomatedTests(ctx context.Context, cwd, projectRoot string, targetLayers ...string) []string {
	fmt.Fprintln(e.Out, "\n=== Running automated tests (build, checks and test suites from the project config) ===")
	if cwd != projectRoot {
		fmt.Fprintf(e.Out, "(in %s)\n", cwd)
	}
	raw, err := projectconfig.ForProject(cwd, projectRoot)
	if err != nil {
		fmt.Fprintln(e.Err, err.Error())
		return []string{"Project config"}
	}
	b, _ := json.Marshal(raw)
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		fmt.Fprintln(e.Err, err.Error())
		return []string{"Project config"}
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) == nil {
		_, cfg.ChecksSet = keys["checks"]
	}

	var proj *projects.Project
	if len(e.Projects) > 0 {
		name := os.Getenv(repo.ProjectNameEnv)
		if name != "" {
			proj = projects.FindProject(e.Projects, name)
		}
		if proj == nil {
			for i := range e.Projects {
				if projects.SamePath(e.Projects[i].Path, projectRoot) || projects.SamePath(e.Projects[i].Path, cwd) {
					proj = &e.Projects[i]
					break
				}
			}
		}
	} else {
		regFile := e.RegistryFile
		if regFile == "" {
			regFile = projects.DefaultRegistryFile()
		}
		if list, _, err := projects.LoadRegistry(regFile); err == nil && len(list) > 0 {
			name := os.Getenv(repo.ProjectNameEnv)
			if name != "" {
				proj = projects.FindProject(list, name)
			}
			if proj == nil {
				for i := range list {
					if projects.SamePath(list[i].Path, projectRoot) || projects.SamePath(list[i].Path, cwd) {
						proj = &list[i]
						break
					}
				}
			}
		}
	}
	if proj != nil && !proj.Tests.IsEmpty() {
		cfg.Tests = &proj.Tests
	}

	failures, _ := e.RunChecks(ctx, e.BuildCheckList(cfg, false, targetLayers...), cwd)
	return failures
}
