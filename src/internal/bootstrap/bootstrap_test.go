package bootstrap_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	wdb "work_dot_bat"
	"work_dot_bat/src/internal/bootstrap"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/projects"
)

func assets(t *testing.T) (vendor, templates fs.FS) {
	t.Helper()
	v, err := fs.Sub(wdb.Vendor, "src/vendor")
	if err != nil {
		t.Fatal(err)
	}
	tm, err := fs.Sub(wdb.Templates, "templates/files")
	if err != nil {
		t.Fatal(err)
	}
	return v, tm
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBootstrapRegistersIntoProjectsJSON(t *testing.T) {
	repo := fakeRepo(t)
	regFile := filepath.Join(t.TempDir(), "projects.json")
	v, tm := assets(t)

	res, err := bootstrap.Run(repo, bootstrap.Options{
		ProjectName:  "TestProj",
		DryRun:       false,
		Vendor:       v,
		Templates:    tm,
		Today:        "2026-10-09",
		RegistryFile: regFile,
	})
	if err != nil {
		t.Fatalf("bootstrap.Run failed: %v", err)
	}
	if !res.ConfigCreated {
		t.Errorf("expected ConfigCreated to be true")
	}

	// Verify work_dot_bat.config.json was NOT created
	l := projectconfig.Load(repo)
	if l.Exists {
		t.Errorf("expected work_dot_bat.config.json not to exist")
	}

	// Verify project was registered in projects.json
	projs, errs, err := projects.LoadRegistry(regFile)
	if err != nil || len(errs) != 0 || len(projs) != 1 {
		t.Fatalf("registry load failed: %v, errs: %v, count: %d", err, errs, len(projs))
	}
	p := projs[0]
	if p.Name != "TestProj" {
		t.Errorf("project name = %q, want TestProj", p.Name)
	}
	if !projects.SamePath(p.Path, repo) {
		t.Errorf("project path = %q, want %q", p.Path, repo)
	}
	if len(p.Build) == 0 {
		t.Errorf("expected default build commands in projects.json")
	}
	if p.Install == nil {
		t.Errorf("expected default install command in projects.json")
	}
	if p.DevServer == nil {
		t.Errorf("expected default devServer command in projects.json")
	}
}

func TestBootstrapUsesExistingProjectsJSON(t *testing.T) {
	repo := fakeRepo(t)
	regFile := filepath.Join(t.TempDir(), "projects.json")
	v, tm := assets(t)

	existing := projects.Project{
		Name:                "CustomProj",
		Path:                repo,
		ProjectStructureDoc: "agent_docs/custom_structure.md",
		Build:               []projects.Command{{Cmd: "go", Args: []string{"build", "./..."}}},
	}
	if err := projects.SaveRegistry(regFile, []projects.Project{existing}); err != nil {
		t.Fatal(err)
	}

	res, err := bootstrap.Run(repo, bootstrap.Options{
		ProjectName:  "CustomProj",
		DryRun:       false,
		Vendor:       v,
		Templates:    tm,
		Today:        "2026-10-09",
		RegistryFile: regFile,
	})
	if err != nil {
		t.Fatalf("bootstrap.Run failed: %v", err)
	}
	if res.ConfigCreated {
		t.Errorf("expected ConfigCreated to be false when config already exists in projects.json")
	}

	// Ensure projects.json was not overwritten with defaults
	projs, _, _ := projects.LoadRegistry(regFile)
	if len(projs) != 1 || projs[0].ProjectStructureDoc != "agent_docs/custom_structure.md" {
		t.Errorf("projects.json altered unexpectedly: %+v", projs)
	}
}
