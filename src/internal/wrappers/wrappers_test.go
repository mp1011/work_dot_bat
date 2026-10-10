package wrappers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesWrappersForEachTool(t *testing.T) {
	repo := t.TempDir()
	files, err := Install(repo, `C:\tools\work_dot_bat`, "my-proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files reported")
	}
	for _, tool := range []string{"work", "auto_daemon", "prepare_for_push"} {
		sh, err := os.ReadFile(filepath.Join(repo, ".tools", tool))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		want := `cd "C:/tools/work_dot_bat" && exec "C:/tools/work_dot_bat/.tools/` + tool + `" project=my-proj "$@"`
		if !strings.HasPrefix(string(sh), "#!/usr/bin/env bash\n") || !strings.Contains(string(sh), want) {
			t.Errorf("%s script:\n%s\nwant line %s", tool, sh, want)
		}
		bat, err := os.ReadFile(filepath.Join(repo, ".tools", tool+".bat"))
		if err != nil {
			t.Fatalf("%s.bat: %v", tool, err)
		}
		wantBat := `cd /d "C:\tools\work_dot_bat" && call "C:\tools\work_dot_bat\.tools\` + tool + `.bat" project=my-proj %*`
		if !strings.Contains(string(bat), wantBat) {
			t.Errorf("%s.bat:\n%s\nwant line %s", tool, bat, wantBat)
		}
	}
}

func TestInstallIsGitIgnoredAndIdempotent(t *testing.T) {
	repo := t.TempDir()
	for i := 0; i < 2; i++ {
		if _, err := Install(repo, "/opt/wdb", "p"); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(repo, ".tools", ".gitignore"))
	if err != nil || strings.TrimSpace(string(ignore)) != "*" {
		t.Errorf("gitignore %q err %v", ignore, err)
	}
}

func TestInstallRewritesStaleWrapper(t *testing.T) {
	repo := t.TempDir()
	Install(repo, "/old", "p")
	Install(repo, "/new", "p")
	sh, _ := os.ReadFile(filepath.Join(repo, ".tools", "work"))
	if strings.Contains(string(sh), "/old") || !strings.Contains(string(sh), "/new/.tools/work") {
		t.Errorf("stale wrapper:\n%s", sh)
	}
}

func TestInstallMissingRepoFails(t *testing.T) {
	if _, err := Install(filepath.Join(t.TempDir(), "nope"), "/x", "p"); err == nil {
		t.Error("want error for a repo directory that does not exist")
	}
}
