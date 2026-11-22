package launcher

import (
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/src/internal/projects"
)

func TestFormatMenu(t *testing.T) {
	cmds := []projects.LaunchCommand{
		{Name: "Dev server", Command: projects.Command{Cmd: "npm", Args: []string{"run", "dev"}}},
		{Name: "Production", Command: projects.Command{Cmd: "npm", Args: []string{"start"}}},
	}
	menu := FormatMenu("my_app", cmds)
	for _, want := range []string{
		"my_app",
		"1) Dev server",
		"npm run dev",
		"2) Production",
		"npm start",
		"a) Add a new launch command",
		"q) Quit",
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("menu missing %q:\n%s", want, menu)
		}
	}
}

func TestRunSelectsAndExecutesCommand(t *testing.T) {
	proj := projects.Project{
		Name: "my_app",
		Path: filepath.Join(t.TempDir(), "my_app"),
		LaunchCommands: []projects.LaunchCommand{
			{Name: "Dev", Command: projects.Command{Cmd: "go", Args: []string{"run", "."}}},
			{Name: "Prod", Command: projects.Command{Cmd: "./app", Args: []string{"--prod"}}},
		},
	}
	var ranCmd projects.Command
	var ranDir string
	var logs []string
	answers := []string{"2"}
	idx := 0

	code := Run(Options{
		Ask: func(prompt string) (string, bool) {
			if idx >= len(answers) {
				return "", false
			}
			a := answers[idx]
			idx++
			return a, true
		},
		Log:     func(msg string) { logs = append(logs, msg) },
		Project: proj,
		Exec: func(cmd projects.Command, dir string) int {
			ranCmd = cmd
			ranDir = dir
			return 42
		},
	})

	if code != 42 {
		t.Errorf("code = %d, want 42", code)
	}
	if ranCmd.Cmd != "./app" || len(ranCmd.Args) != 1 || ranCmd.Args[0] != "--prod" {
		t.Errorf("ranCmd = %+v", ranCmd)
	}
	if ranDir != proj.Path {
		t.Errorf("ranDir = %q, want %q", ranDir, proj.Path)
	}
}

func TestRunAddNewEntryAsksCommandThenDescriptionAndPersists(t *testing.T) {
	regFile := filepath.Join(t.TempDir(), "projects.json")
	repoDir := filepath.Join(t.TempDir(), "my_app")
	proj := projects.Project{
		Name: "my_app",
		Path: repoDir,
	}
	if err := projects.SaveRegistry(regFile, []projects.Project{proj}); err != nil {
		t.Fatal(err)
	}

	var prompts []string
	answers := []string{
		"a",
		"npm run dev -- --port 3000",
		"Local dev server",
		"1",
	}
	idx := 0
	var ranCmd projects.Command

	code := Run(Options{
		Ask: func(prompt string) (string, bool) {
			prompts = append(prompts, prompt)
			if idx >= len(answers) {
				return "", false
			}
			a := answers[idx]
			idx++
			return a, true
		},
		Log:          func(string) {},
		Project:      proj,
		RegistryFile: regFile,
		Exec: func(cmd projects.Command, dir string) int {
			ranCmd = cmd
			return 0
		},
	})

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(prompts) < 3 {
		t.Fatalf("expected at least 3 prompts, got %v", prompts)
	}
	if !strings.Contains(strings.ToLower(prompts[1]), "command") {
		t.Errorf("first add prompt should ask for command, got %q", prompts[1])
	}
	if !strings.Contains(strings.ToLower(prompts[2]), "description") {
		t.Errorf("second add prompt should ask for description, got %q", prompts[2])
	}
	if ranCmd.Cmd != "npm" || strings.Join(ranCmd.Args, " ") != "run dev -- --port 3000" {
		t.Errorf("ranCmd = %+v", ranCmd)
	}

	loaded, errs, err := projects.LoadRegistry(regFile)
	if err != nil || len(errs) > 0 {
		t.Fatalf("LoadRegistry: err=%v errs=%v", err, errs)
	}
	if len(loaded) != 1 || len(loaded[0].LaunchCommands) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
	got := loaded[0].LaunchCommands[0]
	if got.Name != "Local dev server" || got.Command.Cmd != "npm" {
		t.Errorf("saved LaunchCommand = %+v", got)
	}
}

func TestRunQuitAndInvalidChoice(t *testing.T) {
	proj := projects.Project{Name: "my_app", Path: t.TempDir()}
	var logs []string
	answers := []string{"99", "bogus", "q"}
	idx := 0

	code := Run(Options{
		Ask: func(string) (string, bool) {
			if idx >= len(answers) {
				return "", false
			}
			a := answers[idx]
			idx++
			return a, true
		},
		Log:     func(msg string) { logs = append(logs, msg) },
		Project: proj,
		Exec:    func(projects.Command, string) int { t.Fatal("should not exec"); return 1 },
	})

	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "Invalid choice") {
		t.Errorf("expected Invalid choice in logs:\n%s", joined)
	}
}

func TestResolveCommandWindowsShellScripts(t *testing.T) {
	name, args := resolveCommand("windows", projects.Command{
		Cmd:  ".tools/build-local.sh",
		Args: []string{"--fast"},
	}, func() string { return `C:\Program Files\Git\bin\bash.exe` })

	if name != `C:\Program Files\Git\bin\bash.exe` {
		t.Errorf("name = %q, want Git bash.exe", name)
	}
	if len(args) != 2 || args[0] != ".tools/build-local.sh" || args[1] != "--fast" {
		t.Errorf("args = %v", args)
	}

	batName, _ := resolveCommand("windows", projects.Command{
		Cmd: ".tools/build-local.bat",
	}, nil)
	if batName != `.tools\build-local.bat` && batName != `.tools/build-local.bat` {
		t.Errorf("batName = %q", batName)
	}
}
