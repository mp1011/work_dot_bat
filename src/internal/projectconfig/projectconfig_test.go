package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/projects"
)

func parse(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name, json, wantErr string // wantErr "" = valid
	}{
		{"empty object", `{}`, ""},
		{"unknown fields allowed", `{"future": 1}`, ""},
		{"not an object", `[]`, "expected a JSON object"},
		{"good build", `{"build":[{"cmd":"npm","args":["run","lint"]}]}`, ""},
		{"bad build", `{"build":[{"cmd":"","args":[]}]}`, `"build"`},
		{"build not array", `{"build":{"cmd":"x","args":[]}}`, `"build"`},
		{"bad lightTest", `{"lightTest":{"cmd":"x"}}`, `"lightTest"`},
		{"null devServer ok", `{"devServer":null}`, ""},
		{"bad devServer", `{"devServer":"npm"}`, `"devServer"`},
		{"bad env", `{"install":{"cmd":"x","args":[],"env":[]}}`, `"install"`},
		{"bad shell", `{"install":{"cmd":"x","args":[],"shell":"yes"}}`, `"install"`},
		{"unknown check", `{"checks":["nope"]}`, `unknown check id "nope"`},
		{"known check", `{"checks":["monolith"]}`, ""},
		{"bad extraChecks", `{"extraChecks":[{"name":"","command":{"cmd":"x","args":[]}}]}`, `"extraChecks"`},
		{"bad worktreeCopyFiles", `{"worktreeCopyFiles":[1]}`, `"worktreeCopyFiles"`},
		{"bad scanDirs", `{"scanDirs":[1]}`, `"scanDirs"`},
		{"good scanDirs", `{"scanDirs":["a","b"]}`, ""},
		{"bad scanRoots", `{"scanRoots":"not-an-array"}`, `"scanRoots"`},
		{"good scanRoots", `{"scanRoots":["a"]}`, ""},
		{"bad scanExtensions", `{"scanExtensions":[1]}`, `"scanExtensions"`},
		{"good scanExtensions", `{"scanExtensions":[".cs",".fx"]}`, ""},
		{"bad excludeDirs", `{"excludeDirs":[true]}`, `"excludeDirs"`},
		{"good excludeDirs", `{"excludeDirs":["bin","obj"]}`, ""},
		{"bad agents", `{"agents":[]}`, `"agents"`},
		{"bad doc", `{"projectStructureDoc":""}`, `"projectStructureDoc"`},
		{"bad instructions", `{"instructions":{"daemonDoc":3}}`, `"instructions"`},
		{"good instructions", `{"instructions":{"daemonDoc":"x.md"}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(parse(t, tt.json))
			if tt.wantErr == "" && len(errs) > 0 {
				t.Errorf("unexpected errors %v", errs)
			}
			if tt.wantErr != "" && !strings.Contains(strings.Join(errs, "\n"), tt.wantErr) {
				t.Errorf("errors %v lack %q", errs, tt.wantErr)
			}
		})
	}
}

func TestLoadAndForProject(t *testing.T) {
	main, tree := t.TempDir(), t.TempDir()
	if l := Load(main); l.Exists || l.Config != nil || len(l.Errors) != 0 {
		t.Errorf("missing: %+v", l)
	}
	if _, err := ForProject(tree, main); err == nil || !strings.Contains(err.Error(), tree+" or "+main) {
		t.Errorf("want error naming both roots, got %v", err)
	}

	os.WriteFile(filepath.Join(main, ConfigFile), []byte(`{"instructions":{"daemonDoc":"d.md"}}`), 0o644)
	if got := DaemonDoc(tree, main); got != "d.md" { // worktree falls back to the project root
		t.Errorf("DaemonDoc = %q", got)
	}

	os.WriteFile(filepath.Join(tree, ConfigFile), []byte(`{"build":3}`), 0o644)
	if _, err := ForProject(tree, main); err == nil || !strings.Contains(err.Error(), `"build"`) {
		t.Errorf("invalid config: %v", err)
	}
	if got := DaemonDoc(tree, main); got != DefaultDaemonDoc {
		t.Errorf("invalid config should default, got %q", got)
	}

	os.WriteFile(filepath.Join(tree, ConfigFile), []byte(`{nope`), 0o644)
	if l := Load(tree); !l.Exists || len(l.Errors) != 1 || l.Config != nil {
		t.Errorf("unparseable: %+v", l)
	}
}

func TestForProjectPrecedenceFromProjectsJSON(t *testing.T) {
	regDir := t.TempDir()
	regFile := filepath.Join(regDir, "projects.json")
	t.Setenv(projects.RegistryEnv, regFile)

	repoA := t.TempDir()
	repoB := t.TempDir()

	// repoA has config in projects.json AND a legacy work_dot_bat.config.json
	// projects.json should take precedence!
	os.WriteFile(filepath.Join(repoA, ConfigFile), []byte(`{"projectStructureDoc":"legacy.md"}`), 0o644)

	// repoB has no config in projects.json, but has a legacy work_dot_bat.config.json
	os.WriteFile(filepath.Join(repoB, ConfigFile), []byte(`{"projectStructureDoc":"legacy_b.md"}`), 0o644)

	projs := []projects.Project{
		{
			Name:                "projA",
			Path:                repoA,
			ProjectStructureDoc: "consolidated.md",
			Build:               []projects.Command{{Cmd: "npm", Args: []string{"run", "build"}}},
		},
		{
			Name: "projB",
			Path: repoB,
			// No config in projects.json
		},
	}
	if err := projects.SaveRegistry(regFile, projs); err != nil {
		t.Fatal(err)
	}

	// For repoA: should return consolidated.md from projects.json
	cfgA, err := ForProject(repoA, repoA)
	if err != nil {
		t.Fatalf("ForProject(repoA) failed: %v", err)
	}
	if cfgA["projectStructureDoc"] != "consolidated.md" {
		t.Errorf("expected consolidated.md from projects.json, got %v", cfgA["projectStructureDoc"])
	}

	// For repoB: should return legacy_b.md from work_dot_bat.config.json
	cfgB, err := ForProject(repoB, repoB)
	if err != nil {
		t.Fatalf("ForProject(repoB) failed: %v", err)
	}
	if cfgB["projectStructureDoc"] != "legacy_b.md" {
		t.Errorf("expected legacy_b.md from fallback config, got %v", cfgB["projectStructureDoc"])
	}
}

func TestWriteMatchesLoad(t *testing.T) {
	dir := t.TempDir()
	file, err := Write(dir, NewRepoConfig(NpmDefaults()))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(file) != ConfigFile {
		t.Errorf("file = %s", file)
	}
	l := Load(dir)
	if len(l.Errors) != 0 || l.Config["projectStructureDoc"] != "agent_docs/02_project_structure.md" {
		t.Errorf("loaded %+v", l)
	}
}

func TestParseAndFormatCommand(t *testing.T) {
	c := ParseCommandLine("  npm  test -- --run ")
	if c == nil || FormatCommand(*c) != "npm test -- --run" {
		t.Errorf("got %+v", c)
	}
	if ParseCommandLine("   ") != nil {
		t.Error("blank should be nil")
	}
}
