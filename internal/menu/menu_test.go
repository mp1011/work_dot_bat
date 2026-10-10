package menu

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/internal/projectconfig"
	"work_dot_bat/internal/projects"
)

type script struct {
	lines []string
	log   []string
}

func (s *script) opts(registry string, list []projects.Project) Options {
	return Options{
		IO: IO{
			Ask: func(string) (string, bool) {
				if len(s.lines) == 0 {
					return "", false
				}
				l := s.lines[0]
				s.lines = s.lines[1:]
				return l, true
			},
			Log: func(m string) { s.log = append(s.log, m) },
		},
		Projects:     list,
		RegistryFile: registry,
	}
}

func fakeRepo(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunPicksNumberedProject(t *testing.T) {
	s := &script{lines: []string{"bogus", "2"}}
	got := Run(s.opts("", []projects.Project{{Name: "a", Path: "/a"}, {Name: "b", Path: "/b"}}))
	if got == nil || got.Name != "b" {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(strings.Join(s.log, "\n"), "Invalid choice.") {
		t.Error("expected Invalid choice.")
	}
}

func TestRunQuitAndEOF(t *testing.T) {
	for _, lines := range [][]string{{"q"}, {}} {
		if got := Run((&script{lines: lines}).opts("", nil)); got != nil {
			t.Errorf("lines %v: got %v", lines, got)
		}
	}
}

func TestRunPasteRegistersPath(t *testing.T) {
	repo := fakeRepo(t, "newrepo")
	registry := filepath.Join(t.TempDir(), "projects.json")
	// paste, path, default name, then Enter x4 for the command prompts and "none" for the last.
	s := &script{lines: []string{"p", repo, "", "", "", "", "", "none"}}
	got := Run(s.opts(registry, []projects.Project{{Name: "a", Path: "/a"}}))
	if got == nil || got.Name != "newrepo" || got.Path != repo {
		t.Fatalf("got %v", got)
	}
	saved, errs, err := projects.LoadRegistry(registry)
	if err != nil || len(errs) != 0 || len(saved) != 2 || saved[1].Name != "newrepo" {
		t.Errorf("registry = %v %v %v", saved, errs, err)
	}
	l := projectconfig.Load(repo)
	if l.Exists {
		t.Error("expected no work_dot_bat.config.json to be written")
	}
	regProj := saved[1]
	if regProj.DevServer != nil {
		t.Error(`"none" should omit devServer`)
	}
	if len(regProj.Build) == 0 {
		t.Error("Enter should accept the build default")
	}
	if regProj.Install == nil {
		t.Error("Enter should accept the install default")
	}
	if len(regProj.Tests.Light) == 0 {
		t.Error("Enter should accept light test default")
	}
	if len(regProj.Tests.Heavy) == 0 {
		t.Error("Enter should accept heavy test default")
	}
}

func TestPasteRepromptsOnBadInputAndCancels(t *testing.T) {
	good := fakeRepo(t, "dup")
	notRepo := t.TempDir()
	s := &script{lines: []string{
		notRepo,               // not a git repo -> re-prompt
		good, "bad name", "x", // invalid name -> re-prompt; "x" collides with the registered name below
		"", // blank path -> cancel
	}}
	got := RegisterPastedPath(s.opts("", []projects.Project{{Name: "x", Path: "/x"}}))
	if got != nil {
		t.Fatalf("got %v", got)
	}
	log := strings.Join(s.log, "\n")
	for _, want := range []string{"no .git", "Names must be non-empty", `a project named "x" is already registered`} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
}

func TestPasteUsesExistingConfigAndOffersBootstrap(t *testing.T) {
	repo := fakeRepo(t, "r")
	cfg := `{"build":[]}`
	os.WriteFile(filepath.Join(repo, projectconfig.ConfigFile), []byte(cfg), 0o644)
	registry := filepath.Join(t.TempDir(), "projects.json")
	s := &script{lines: []string{`"` + repo + `"`, "custom", "y"}}
	o := s.opts(registry, nil)
	var bootstrapped string
	o.Bootstrap = func(p projects.Project) (string, error) { bootstrapped = p.Name; return "report", nil }
	got := RegisterPastedPath(o)
	if got == nil || got.Name != "custom" || bootstrapped != "custom" {
		t.Fatalf("got %v, bootstrapped %q", got, bootstrapped)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, projectconfig.ConfigFile)); string(b) != cfg {
		t.Error("existing config must not be rewritten")
	}
	if !strings.Contains(strings.Join(s.log, "\n"), "Using the existing") {
		t.Error("expected existing-config notice")
	}
}

func TestPasteWritesToolWrappers(t *testing.T) {
	repo := fakeRepo(t, "r")
	os.WriteFile(filepath.Join(repo, projectconfig.ConfigFile), []byte(`{"build":[]}`), 0o644)
	s := &script{lines: []string{repo, "custom"}}
	o := s.opts(filepath.Join(t.TempDir(), "projects.json"), nil)
	o.ToolRoot = "/opt/wdb"
	if got := RegisterPastedPath(o); got == nil {
		t.Fatal("not registered")
	}
	sh, err := os.ReadFile(filepath.Join(repo, ".tools", "work"))
	if err != nil || !strings.Contains(string(sh), "project=custom") {
		t.Errorf("wrapper %q err %v", sh, err)
	}
}
