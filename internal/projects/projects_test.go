package projects

import (
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
	if !reflect.DeepEqual(got, []Project{{"good", "/a"}}) {
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
	in := []Project{{"foo", "/foo"}, {"bar", "/bar"}}
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
	list := []Project{{"foo", "/foo"}, {"bar", "/bar"}}
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
	orig := []Project{{"foo", `C:\Src\Foo`}}
	got, err := AddProject(orig, Project{"bar", "/bar"})
	if err != nil || len(got) != 2 || len(orig) != 1 {
		t.Errorf("add: %v %v (orig %v)", got, err, orig)
	}
	for _, p := range []Project{{"foo", "/x"}, {"x", "c:/src/foo/"}, {"a b", "/y"}} {
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
	list := []Project{{"a", "/a"}, {"long", "/long"}}
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
