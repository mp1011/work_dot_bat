package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseProjectArg(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		wantName *string
		wantRest []string
	}{
		{"extracts and keeps order", []string{"a", "project=foo", "--b"}, ptr("foo"), []string{"a", "--b"}},
		{"absent", []string{"a"}, nil, []string{"a"}},
		{"last wins", []string{"project=a", "project=b"}, ptr("b"), nil},
		{"empty value", []string{"project="}, ptr(""), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, rest := ParseProjectArg(tt.argv)
			if (name == nil) != (tt.wantName == nil) || (name != nil && *name != *tt.wantName) {
				t.Errorf("name = %v, want %v", name, tt.wantName)
			}
			if !reflect.DeepEqual(rest, tt.wantRest) {
				t.Errorf("rest = %v, want %v", rest, tt.wantRest)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestIsValidProjectName(t *testing.T) {
	for name, want := range map[string]bool{"ok": true, "a-b_c": true, "": false, "a b": false, "a=b": false, "a\tb": false} {
		if got := IsValidProjectName(name); got != want {
			t.Errorf("IsValidProjectName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDefaultProjectName(t *testing.T) {
	for in, want := range map[string]string{
		`C:\src\repos\foo`: "foo", "/home/u/foo/": "foo", `C:\src\foo\\`: "foo", "foo": "foo",
	} {
		if got := DefaultProjectName(in); got != want {
			t.Errorf("DefaultProjectName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateRegistry(t *testing.T) {
	data := map[string]any{"projects": []any{
		map[string]any{"name": "good", "path": "/a"},
		map[string]any{"name": "bad name", "path": "/b"},
		map[string]any{"name": "nopath"},
		map[string]any{"name": "good", "path": "/c"},
		"str",
	}}
	got, errs := ValidateRegistry(data)
	if !reflect.DeepEqual(got, []Project{{Name: "good", Path: "/a"}}) {
		t.Errorf("projects = %v", got)
	}
	want := []string{`"bad name"`, `project "nopath": "path"`, `project "good": duplicate "name"`, "projects[4]: expected an object"}
	if len(errs) != len(want) {
		t.Fatalf("errors = %v", errs)
	}
	for i, w := range want {
		if !strings.Contains(errs[i], w) {
			t.Errorf("errs[%d] = %q, want it to contain %q", i, errs[i], w)
		}
	}
	if _, errs := ValidateRegistry([]any{}); len(errs) != 1 {
		t.Errorf("non-object registry errors = %v", errs)
	}
}

func TestLoadSaveRegistry(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "projects.json")
	if got, errs, err := LoadRegistry(file); got != nil || errs != nil || err != nil {
		t.Errorf("missing file: %v %v %v", got, errs, err)
	}
	in := []Project{{Name: "foo", Path: "/foo"}, {Name: "bar", Path: "/bar"}}
	if err := SaveRegistry(file, in); err != nil {
		t.Fatal(err)
	}
	got, errs, err := LoadRegistry(file)
	if err != nil || len(errs) != 0 || !reflect.DeepEqual(got, in) {
		t.Errorf("round trip: %v %v %v", got, errs, err)
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{nope"), 0o644)
	if _, _, err := LoadRegistry(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Errorf("want error naming file, got %v", err)
	}
}

func TestResolveProject(t *testing.T) {
	list := []Project{{Name: "foo", Path: "/foo"}, {Name: "bar", Path: "/bar"}}
	p, err := ResolveProject(list, "foo")
	if err != nil || p.Path != "/foo" {
		t.Errorf("foo: %v %v", p, err)
	}
	_, err = ResolveProject(list, "nope")
	if err == nil || !strings.Contains(err.Error(), "foo, bar") || !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("unknown: %v", err)
	}
	if _, err = ResolveProject(nil, "x"); err == nil || !strings.Contains(err.Error(), "(none registered)") {
		t.Errorf("empty: %v", err)
	}
}

func TestAddProject(t *testing.T) {
	orig := []Project{{Name: "foo", Path: `C:\Src\Foo`}}
	got, err := AddProject(orig, Project{Name: "bar", Path: "/bar"})
	if err != nil || len(got) != 2 || len(orig) != 1 {
		t.Errorf("add: %v %v (orig %v)", got, err, orig)
	}
	for _, p := range []Project{{Name: "foo", Path: "/x"}, {Name: "x", Path: "c:/src/foo/"}, {Name: "a b", Path: "/y"}} {
		if _, err := AddProject(orig, p); err == nil {
			t.Errorf("AddProject(%v) should fail", p)
		}
	}
}

func TestCheckRepoRoot(t *testing.T) {
	dir := t.TempDir()
	if ok, reason := CheckRepoRoot(filepath.Join(dir, "missing")); ok || !strings.Contains(reason, "does not exist") {
		t.Errorf("missing: %v %q", ok, reason)
	}
	if ok, reason := CheckRepoRoot(dir); ok || !strings.Contains(reason, "no .git") {
		t.Errorf("no .git: %v %q", ok, reason)
	}
	os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: x"), 0o644) // a worktree has a .git file
	if ok, _ := CheckRepoRoot(dir); !ok {
		t.Error(".git file should be accepted")
	}
}

func TestFormatMenuAndParseChoice(t *testing.T) {
	list := []Project{{Name: "a", Path: "/a"}, {Name: "long", Path: "/long"}}
	menu := FormatMenu(list)
	want := "Select a project:\n  1) a      /a\n  2) long   /long\n  p) Paste a repository path\n  q) Quit"
	if menu != want {
		t.Errorf("menu:\n%s\nwant:\n%s", menu, want)
	}
	tests := []struct {
		in   string
		kind ChoiceKind
		name string
	}{
		{"2", ChoiceProject, "long"}, {" 1 \n", ChoiceProject, "a"}, {"P", ChoicePaste, ""}, {"q", ChoiceQuit, ""},
		{"0", ChoiceInvalid, ""}, {"3", ChoiceInvalid, ""}, {"x", ChoiceInvalid, ""}, {"", ChoiceInvalid, ""},
	}
	for _, tt := range tests {
		c := ParseMenuChoice(tt.in, list)
		if c.Kind != tt.kind || c.Project.Name != tt.name {
			t.Errorf("ParseMenuChoice(%q) = %+v", tt.in, c)
		}
	}
}

func TestDefaultRegistryFileHonoursEnv(t *testing.T) {
	t.Setenv(RegistryEnv, "/custom/projects.json")
	if got := DefaultRegistryFile(); got != "/custom/projects.json" {
		t.Errorf("got %q", got)
	}
	t.Setenv(RegistryEnv, "")
	if got := DefaultRegistryFile(); filepath.Base(got) != "projects.json" {
		t.Errorf("got %q", got)
	}
}

func TestValidateRegistryTestsConfig(t *testing.T) {
	t.Run("valid full test config", func(t *testing.T) {
		data := map[string]any{"projects": []any{
			map[string]any{
				"name": "full_tests",
				"path": "/repo",
				"tests": map[string]any{
					"ultra_light": []any{"golangci-lint", "run"},
					"light":       []any{"go", "test", "./..."},
					"heavy":       []any{"go", "test", "-tags=integration", "./..."},
					"ultra_heavy": []any{"scripts/run-checks.sh"},
				},
			},
		}}
		got, errs := ValidateRegistry(data)
		if len(errs) != 0 {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
		p := FindProject(got, "full_tests")
		if p == nil {
			t.Fatalf("project full_tests not found")
		}
		wantTests := TestsConfig{
			UltraLight: CommandList{{"golangci-lint", "run"}},
			Light:      CommandList{{"go", "test", "./..."}},
			Heavy:      CommandList{{"go", "test", "-tags=integration", "./..."}},
			UltraHeavy: CommandList{{"scripts/run-checks.sh"}},
		}
		if !reflect.DeepEqual(p.Tests, wantTests) {
			t.Errorf("p.Tests = %+v, want %+v", p.Tests, wantTests)
		}
	})

	t.Run("valid multi-command test config", func(t *testing.T) {
		data := map[string]any{"projects": []any{
			map[string]any{
				"name": "multi_tests",
				"path": "/repo",
				"tests": map[string]any{
					"ultra_light": []any{
						[]any{"npm", "run", "lint"},
						[]any{"npx", "tsc", "--noEmit"},
					},
					"light": []any{"go", "test", "./..."},
				},
			},
		}}
		got, errs := ValidateRegistry(data)
		if len(errs) != 0 {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
		p := FindProject(got, "multi_tests")
		if p == nil {
			t.Fatalf("project multi_tests not found")
		}
		wantUltraLight := CommandList{{"npm", "run", "lint"}, {"npx", "tsc", "--noEmit"}}
		if !reflect.DeepEqual(p.Tests.UltraLight, wantUltraLight) {
			t.Errorf("p.Tests.UltraLight = %+v, want %+v", p.Tests.UltraLight, wantUltraLight)
		}
		wantLight := CommandList{{"go", "test", "./..."}}
		if !reflect.DeepEqual(p.Tests.Light, wantLight) {
			t.Errorf("p.Tests.Light = %+v, want %+v", p.Tests.Light, wantLight)
		}
	})

	t.Run("valid empty test block", func(t *testing.T) {
		data := map[string]any{"projects": []any{
			map[string]any{
				"name":  "empty_tests",
				"path":  "/repo",
				"tests": map[string]any{},
			},
		}}
		got, errs := ValidateRegistry(data)
		if len(errs) != 0 {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
		p := FindProject(got, "empty_tests")
		if p == nil {
			t.Fatalf("project empty_tests not found")
		}
		if !p.Tests.IsEmpty() {
			t.Errorf("expected empty TestsConfig, got %+v", p.Tests)
		}
	})

	t.Run("valid partial test block", func(t *testing.T) {
		data := map[string]any{"projects": []any{
			map[string]any{
				"name": "partial_tests",
				"path": "/repo",
				"tests": map[string]any{
					"light": []any{"go", "test", "./..."},
				},
			},
		}}
		got, errs := ValidateRegistry(data)
		if len(errs) != 0 {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
		p := FindProject(got, "partial_tests")
		if p == nil {
			t.Fatalf("project partial_tests not found")
		}
		wantLight := CommandList{{"go", "test", "./..."}}
		if !reflect.DeepEqual(p.Tests.Light, wantLight) {
			t.Errorf("p.Tests.Light = %v, want %v", p.Tests.Light, wantLight)
		}
		if len(p.Tests.UltraLight) != 0 || len(p.Tests.Heavy) != 0 || len(p.Tests.UltraHeavy) != 0 {
			t.Errorf("expected other tiers to be empty, got %+v", p.Tests)
		}
	})

	t.Run("malformed test entries", func(t *testing.T) {
		cases := []struct {
			name        string
			testsVal    any
			wantContain []string
		}{
			{
				name:        "non-object tests string",
				testsVal:    "not an object",
				wantContain: []string{`project "bad"`, `"tests" must be an object`},
			},
			{
				name:        "non-object tests number",
				testsVal:    123,
				wantContain: []string{`project "bad"`, `"tests" must be an object`},
			},
			{
				name:        "non-array layer",
				testsVal:    map[string]any{"light": "not-an-array"},
				wantContain: []string{`project "bad"`, `"tests.light" must be an array of strings`},
			},
			{
				name:        "array containing non-strings",
				testsVal:    map[string]any{"heavy": []any{"go", "test", 123}},
				wantContain: []string{`project "bad"`, `"tests.heavy" must be an array of strings`},
			},
			{
				name:        "empty command name array",
				testsVal:    map[string]any{"ultra_light": []any{}},
				wantContain: []string{`project "bad"`, `"tests.ultra_light" command name must not be empty`},
			},
			{
				name:        "empty command name string",
				testsVal:    map[string]any{"ultra_heavy": []any{"   ", "arg"}},
				wantContain: []string{`project "bad"`, `"tests.ultra_heavy" command name must not be empty`},
			},
			{
				name:        "unknown test layer",
				testsVal:    map[string]any{"custom_tier": []any{"cmd", "arg"}},
				wantContain: []string{`project "bad"`, `"tests.custom_tier": unknown test layer`},
			},
			{
				name:        "multi-command non-array element",
				testsVal:    map[string]any{"ultra_light": []any{[]any{"npm", "run", "lint"}, "not-array"}},
				wantContain: []string{`project "bad"`, `"tests.ultra_light" must be an array of strings`},
			},
			{
				name:        "multi-command empty sub-command array",
				testsVal:    map[string]any{"ultra_light": []any{[]any{"npm", "run", "lint"}, []any{}}},
				wantContain: []string{`project "bad"`, `"tests.ultra_light" command name must not be empty`},
			},
			{
				name:        "multi-command non-string element in sub-command",
				testsVal:    map[string]any{"ultra_light": []any{[]any{"npm", "run", "lint"}, []any{"npx", 123}}},
				wantContain: []string{`project "bad"`, `"tests.ultra_light" must be an array of strings`},
			},
			{
				name:        "multi-command empty command name in sub-command",
				testsVal:    map[string]any{"ultra_light": []any{[]any{"npm", "run", "lint"}, []any{"", "arg"}}},
				wantContain: []string{`project "bad"`, `"tests.ultra_light" command name must not be empty`},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				data := map[string]any{"projects": []any{
					map[string]any{
						"name":  "bad",
						"path":  "/repo",
						"tests": tc.testsVal,
					},
				}}
				got, errs := ValidateRegistry(data)
				if len(got) != 0 {
					t.Errorf("expected entry to be dropped, got %v", got)
				}
				if len(errs) == 0 {
					t.Fatalf("expected validation errors, got none")
				}
				joined := strings.Join(errs, "; ")
				for _, w := range tc.wantContain {
					if !strings.Contains(joined, w) {
						t.Errorf("error %q does not contain %q", joined, w)
					}
				}
			})
		}
	})
}

func TestLoadSaveRegistryWithTests(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "projects.json")

	in := []Project{
		{
			Name: "proj_a",
			Path: "/path/a",
			Tests: TestsConfig{
				UltraLight: CommandList{{"lint", "-v"}, {"tsc", "--noEmit"}},
				Light:      CommandList{{"go", "test", "./..."}},
				Heavy:      CommandList{{"go", "test", "-tags=heavy"}},
				UltraHeavy: CommandList{{"gate.sh"}},
			},
		},
		{
			Name: "proj_b",
			Path: "/path/b",
			// No tests configured
		},
	}

	if err := SaveRegistry(file, in); err != nil {
		t.Fatalf("SaveRegistry failed: %v", err)
	}

	got, errs, err := LoadRegistry(file)
	if err != nil {
		t.Fatalf("LoadRegistry returned error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("LoadRegistry returned validation errors: %v", errs)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round trip mismatch:\ngot:  %+v\nwant: %+v", got, in)
	}

	// Verify that projects without tests do not have a "tests" key emitted in the JSON
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var raw map[string][]map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal raw failed: %v", err)
	}
	for _, p := range raw["projects"] {
		if p["name"] == "proj_b" {
			if _, exists := p["tests"]; exists {
				t.Errorf("proj_b should not have 'tests' field in raw JSON, but found %v", p["tests"])
			}
		}
		if p["name"] == "proj_a" {
			if _, exists := p["tests"]; !exists {
				t.Errorf("proj_a should have 'tests' field in raw JSON, but it was missing")
			}
		}
	}
}

func TestProjectsExampleJSON(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		repoRoot = filepath.Clean(filepath.Join(wd, "..", ".."))
	}
	examplePath := filepath.Join(repoRoot, "projects.example.json")
	projects, errs, err := LoadRegistry(examplePath)
	if err != nil {
		t.Fatalf("LoadRegistry(%q) failed: %v", examplePath, err)
	}
	if len(errs) != 0 {
		t.Fatalf("validation errors in %s: %v", examplePath, errs)
	}
	if len(projects) == 0 {
		t.Fatalf("no projects found in %s", examplePath)
	}
	p := projects[0]
	if p.Tests.IsEmpty() {
		t.Errorf("expected projects.example.json to have tests configured")
	}
	if len(p.Tests.UltraLight) == 0 || len(p.Tests.Light) == 0 || len(p.Tests.Heavy) == 0 || len(p.Tests.UltraHeavy) == 0 {
		t.Errorf("expected all 4 test tiers in projects.example.json, got %+v", p.Tests)
	}
}

func TestValidateRegistryProjectConfig(t *testing.T) {
	data := map[string]any{"projects": []any{
		map[string]any{
			"name": "full",
			"path": "/full",
			"build": []any{
				map[string]any{"cmd": "npm", "args": []any{"run", "lint"}},
				[]any{"npx", "tsc", "--noEmit"},
			},
			"install":              []any{"npm", "install"},
			"devServer":            map[string]any{"cmd": "npm", "args": []any{"run", "dev"}},
			"checks":               []any{"monolith", "duplication"},
			"extraChecks":          []any{map[string]any{"name": "custom", "command": []any{"echo", "hi"}}},
			"worktreeCopyFiles":    []any{".env.local"},
			"projectStructureDoc":  "docs/structure.md",
			"projectStructureDirs": []any{"src", "lib"},
			"agents":               map[string]any{"claude": map[string]any{"model": "sonnet"}},
			"planTemplate":         "docs/template.md",
			"instructions":         map[string]any{"daemonDoc": "daemon.md"},
		},
	}}
	projects, errs := ValidateRegistry(data)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	p := projects[0]
	if len(p.Build) != 2 || p.Build[0].Cmd != "npm" || p.Build[1].Cmd != "npx" {
		t.Errorf("build = %+v", p.Build)
	}
	if p.Install == nil || p.Install.Cmd != "npm" {
		t.Errorf("install = %+v", p.Install)
	}
	if p.DevServer == nil || p.DevServer.Cmd != "npm" {
		t.Errorf("devServer = %+v", p.DevServer)
	}
	if len(p.Checks) != 2 || p.Checks[0] != "monolith" {
		t.Errorf("checks = %+v", p.Checks)
	}
	if len(p.ExtraChecks) != 1 || p.ExtraChecks[0].Name != "custom" {
		t.Errorf("extraChecks = %+v", p.ExtraChecks)
	}
	if len(p.WorktreeCopyFiles) != 1 || p.WorktreeCopyFiles[0] != ".env.local" {
		t.Errorf("worktreeCopyFiles = %+v", p.WorktreeCopyFiles)
	}
	if p.ProjectStructureDoc != "docs/structure.md" {
		t.Errorf("projectStructureDoc = %q", p.ProjectStructureDoc)
	}
	if len(p.ProjectStructureDirs) != 2 {
		t.Errorf("projectStructureDirs = %+v", p.ProjectStructureDirs)
	}
}

func TestValidateRegistryConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		entry   map[string]any
		wantErr string
	}{
		{"bad build", map[string]any{"name": "p", "path": "/p", "build": "npm run lint"}, `"build"`},
		{"bad install", map[string]any{"name": "p", "path": "/p", "install": 123}, `"install"`},
		{"bad devServer", map[string]any{"name": "p", "path": "/p", "devServer": true}, `"devServer"`},
		{"bad checks non-array", map[string]any{"name": "p", "path": "/p", "checks": "monolith"}, `"checks"`},
		{"unknown check id", map[string]any{"name": "p", "path": "/p", "checks": []any{"bogus"}}, `unknown check id "bogus"`},
		{"bad extraChecks non-array", map[string]any{"name": "p", "path": "/p", "extraChecks": "check"}, `"extraChecks"`},
		{"bad extraChecks item", map[string]any{"name": "p", "path": "/p", "extraChecks": []any{map[string]any{"name": ""}}}, `"extraChecks"`},
		{"bad worktreeCopyFiles", map[string]any{"name": "p", "path": "/p", "worktreeCopyFiles": []any{123}}, `"worktreeCopyFiles"`},
		{"bad projectStructureDoc", map[string]any{"name": "p", "path": "/p", "projectStructureDoc": ""}, `"projectStructureDoc"`},
		{"bad projectStructureDirs", map[string]any{"name": "p", "path": "/p", "projectStructureDirs": []any{123}}, `"projectStructureDirs"`},
		{"bad agents", map[string]any{"name": "p", "path": "/p", "agents": []any{"claude"}}, `"agents"`},
		{"bad planTemplate", map[string]any{"name": "p", "path": "/p", "planTemplate": ""}, `"planTemplate"`},
		{"bad instructions", map[string]any{"name": "p", "path": "/p", "instructions": 123}, `"instructions"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{"projects": []any{tt.entry}}
			_, errs := ValidateRegistry(data)
			if len(errs) == 0 {
				t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e, tt.wantErr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

func TestHasConfigAndToConfigMap(t *testing.T) {
	empty := Project{Name: "p", Path: "/p"}
	if empty.HasConfig() {
		t.Errorf("empty project should not have config")
	}

	withBuild := Project{
		Name:  "p",
		Path:  "/p",
		Build: []Command{{Cmd: "npm", Args: []string{"run", "build"}}},
		Tests: TestsConfig{
			Light: CommandList{[]string{"npm", "test"}},
		},
	}
	if !withBuild.HasConfig() {
		t.Errorf("project with build should have config")
	}
	m := withBuild.ToConfigMap()
	if _, ok := m["name"]; ok {
		t.Errorf("ToConfigMap should not have 'name'")
	}
	if _, ok := m["path"]; ok {
		t.Errorf("ToConfigMap should not have 'path'")
	}
	if _, ok := m["build"]; !ok {
		t.Errorf("ToConfigMap missing 'build'")
	}
	if lt, ok := m["lightTest"].(map[string]any); !ok || lt["cmd"] != "npm" {
		t.Errorf("ToConfigMap lightTest = %+v", m["lightTest"])
	}
}

func TestToolRoot(t *testing.T) {
	root := ToolRoot()
	if root == "" || root == "." {
		t.Errorf("ToolRoot() returned %q", root)
	}
	if filepath.Base(root) == "dist" {
		t.Errorf("ToolRoot() must not be dist: %q", root)
	}
}



