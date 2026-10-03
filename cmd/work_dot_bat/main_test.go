package main

import (
	"bytes"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/internal/projects"
	"work_dot_bat/internal/tools"
)

func setup(t *testing.T, list []projects.Project) (registry string, repoDir string) {
	t.Helper()
	repoDir = t.TempDir()
	registry = filepath.Join(t.TempDir(), "projects.json")
	for i := range list {
		if list[i].Path == "" {
			list[i].Path = repoDir
		}
	}
	if err := projects.SaveRegistry(registry, list); err != nil {
		t.Fatal(err)
	}
	return registry, repoDir
}

func exec(t *testing.T, argv []string, stdin string, interactive bool, registry string) (code int, stdout, stderr string) {
	t.Helper()
	t.Chdir(t.TempDir()) // Activate changes directory; restore afterwards
	var out, errb bytes.Buffer
	code = run(argv, env{strings.NewReader(stdin), &out, &errb, interactive, registry})
	return code, out.String(), errb.String()
}

func TestUsageError(t *testing.T) {
	for _, argv := range [][]string{nil, {"nosuchtool"}} {
		code, _, stderr := exec(t, argv, "", false, "")
		if code != 2 || !strings.Contains(stderr, "Usage: work_dot_bat <work|") {
			t.Errorf("%v: code %d stderr %q", argv, code, stderr)
		}
	}
}

func TestUnknownProjectNamesRegistered(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "foo"}, {Name: "bar", Path: "/bar"}})
	code, _, stderr := exec(t, []string{"work", "project=nope"}, "", false, registry)
	if code == 0 || !strings.Contains(stderr, `"nope"`) || !strings.Contains(stderr, "foo, bar") {
		t.Errorf("code %d stderr %q", code, stderr)
	}
}

func TestUnportedToolExitsNonZero(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "foo"}})
	defer tools.Replace("bootstrap", nil)() // a known tool with no Run is "not yet ported"
	code, stdout, stderr := exec(t, []string{"bootstrap", "project=foo"}, "", false, registry)
	if code == 0 || !strings.Contains(stderr, "not yet ported") {
		t.Errorf("code %d stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Project: foo (") {
		t.Errorf("stdout %q", stdout)
	}
}

func TestRegisteredToolReceivesRestArgs(t *testing.T) {
	registry, repoDir := setup(t, []projects.Project{{Name: "foo"}})
	var gotArgs []string
	var gotWd string
	defer tools.Replace("sync", func(args []string) int {
		gotArgs = args
		gotWd, _ = os.Getwd()
		return 7
	})()
	code, _, _ := exec(t, []string{"sync", "--apply", "project=foo", "x"}, "", false, registry)
	if code != 7 || strings.Join(gotArgs, ",") != "--apply,x" {
		t.Errorf("code %d args %v", code, gotArgs)
	}
	if a, _ := filepath.EvalSymlinks(gotWd); !strings.EqualFold(a, mustEval(t, repoDir)) {
		t.Errorf("cwd %s, want %s", gotWd, repoDir)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNoProjectNeedsATerminal(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "foo"}})
	code, _, stderr := exec(t, []string{"work"}, "1\n", false, registry)
	if code == 0 || !strings.Contains(stderr, "Pass project=<name>") || !strings.Contains(stderr, "foo") {
		t.Errorf("code %d stderr %q", code, stderr)
	}
}

func TestMenuSelectsSecondProjectFromStdin(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "a", Path: "/a"}, {Name: "b"}})
	defer tools.Replace("bootstrap", nil)()
	code, stdout, stderr := exec(t, []string{"bootstrap"}, "2\n", true, registry)
	if !strings.Contains(stdout, "Project: b (") || !strings.Contains(stderr, "not yet ported") || code == 0 {
		t.Errorf("code %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
}

func TestMenuPasteRegistersInRegistryFile(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "a", Path: "/a"}})
	newRepo := filepath.Join(t.TempDir(), "fresh")
	os.MkdirAll(filepath.Join(newRepo, ".git"), 0o755)
	os.WriteFile(filepath.Join(newRepo, "work_dot_bat.config.json"), []byte("{}"), 0o644)
	defer tools.Replace("bootstrap", nil)()
	exec(t, []string{"bootstrap"}, "p\n"+newRepo+"\n\n", true, registry)
	got, _, err := projects.LoadRegistry(registry)
	if err != nil || len(got) != 2 || got[1].Name != "fresh" || got[1].Path != newRepo {
		t.Errorf("registry = %v %v", got, err)
	}
}

func TestMenuQuitExitsZero(t *testing.T) {
	registry, _ := setup(t, []projects.Project{{Name: "a", Path: "/a"}})
	if code, _, _ := exec(t, []string{"work"}, "q\n", true, registry); code != 0 {
		t.Errorf("code %d", code)
	}
}

func TestPlanStatusRunsWithoutProject(t *testing.T) {
	repoDir := t.TempDir()
	if out, err := osexec.Command("git", "-C", repoDir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	plan := filepath.Join(repoDir, "agent_docs", "plans", "a.md")
	os.MkdirAll(filepath.Dir(plan), 0o755)
	os.WriteFile(plan, []byte("---\nstatus: Not Started\n---\n\nx\n"), 0o644)
	t.Chdir(repoDir)
	var out, errb bytes.Buffer
	code := run([]string{"plan-status", "list"}, env{strings.NewReader(""), &out, &errb, false, ""})
	if code != 0 || out.String() != "Not Started\tplans/a.md\n" {
		t.Errorf("code %d out %q err %q", code, out.String(), errb.String())
	}
}

func TestListTasksAliasesPlanStatus(t *testing.T) {
	repoDir := t.TempDir()
	if out, err := osexec.Command("git", "-C", repoDir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	plan := filepath.Join(repoDir, "agent_docs", "plans", "a.md")
	os.MkdirAll(filepath.Dir(plan), 0o755)
	os.WriteFile(plan, []byte("---\nstatus: Not Started\n---\n\nx\n"), 0o644)
	t.Chdir(repoDir)
	var out, errb bytes.Buffer
	code := run([]string{"list-tasks"}, env{strings.NewReader(""), &out, &errb, false, ""})
	if code != 0 || out.String() != "1. [Not Started] plans/a.md\n" {
		t.Errorf("code %d out %q err %q", code, out.String(), errb.String())
	}
}
