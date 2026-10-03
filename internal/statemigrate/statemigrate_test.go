package statemigrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(root, rel string) string {
	b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	return string(b)
}

func hasFile(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestNothingToMigrate(t *testing.T) {
	root := t.TempDir()
	plan := PlanMigration(root)
	if len(plan.Moves)+len(plan.Skipped)+len(plan.Dead) != 0 {
		t.Errorf("%+v", plan)
	}
	if got := FormatReport(plan, false, root, false); got != "Nothing to migrate: no legacy state found." {
		t.Errorf("%q", got)
	}
}

func TestApplyMovesLedgerLogsAndClaims(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".tools/session_ledger.json", `[{"sessionName":"s"}]`)
	write(t, root, ".tools/logs/a.log", "log a")
	write(t, root, ".claims/plan--x.json", `{"scope":"plan"}`)
	write(t, root, ".tools/other.txt", "unrelated")
	write(t, root, ".tools/auto_daemon_state.json", "{}")

	plan := PlanMigration(root)
	if len(plan.Moves) != 3 || len(plan.Dead) != 1 {
		t.Fatalf("%+v", plan)
	}
	dry := FormatReport(plan, false, root, false)
	for _, want := range []string{
		"Would move: .tools/session_ledger.json -> .work_dot_bat/session_ledger.json",
		"Would move: .tools/logs -> .work_dot_bat/logs",
		"Would move: .claims -> .work_dot_bat/claims",
		"Not migrated (obsolete, safe to delete): .tools/auto_daemon_state.json",
		"Dry run. Re-run with --apply to move the state. Stop any running daemon first.",
		`Note: add ".work_dot_bat/" to this repo's .gitignore.`,
	} {
		if !strings.Contains(dry, want) {
			t.Errorf("dry run missing %q:\n%s", want, dry)
		}
	}
	if hasFile(root, ".work_dot_bat") {
		t.Fatal("a dry run must not move anything")
	}

	done, err := Apply(plan, root)
	if err != nil || len(done) != 3 {
		t.Fatalf("%v %v", done, err)
	}
	if read(root, ".work_dot_bat/session_ledger.json") != `[{"sessionName":"s"}]` || read(root, ".work_dot_bat/logs/a.log") != "log a" ||
		read(root, ".work_dot_bat/claims/plan--x.json") != `{"scope":"plan"}` {
		t.Error("content not preserved")
	}
	if hasFile(root, ".claims") || hasFile(root, ".tools/logs") || hasFile(root, ".tools/session_ledger.json") {
		t.Error("old locations should be gone")
	}
	if read(root, ".tools/other.txt") != "unrelated" || !hasFile(root, ".tools/auto_daemon_state.json") {
		t.Error("unrelated and obsolete files stay put")
	}
	applied := FormatReport(plan, true, root, true)
	if !strings.Contains(applied, "Moved: .claims -> .work_dot_bat/claims") || strings.Contains(applied, "Dry run") || strings.Contains(applied, "gitignore") {
		t.Errorf("%s", applied)
	}
}

func TestExistingNewLocationIsNeverOverwritten(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".tools/session_ledger.json", "old")
	write(t, root, ".work_dot_bat/session_ledger.json", "new")
	plan := PlanMigration(root)
	if len(plan.Moves) != 0 || len(plan.Skipped) != 1 {
		t.Fatalf("%+v", plan)
	}
	Apply(plan, root)
	if read(root, ".work_dot_bat/session_ledger.json") != "new" || read(root, ".tools/session_ledger.json") != "old" {
		t.Error("both must be untouched")
	}
	if got := FormatReport(plan, false, root, true); !strings.Contains(got, "Skipped: .tools/session_ledger.json (.work_dot_bat/session_ledger.json already exists — left in place)") {
		t.Errorf("%s", got)
	}
}

func TestGitignoreRecognisesCommonSpellings(t *testing.T) {
	for _, line := range []string{".work_dot_bat", ".work_dot_bat/", "/.work_dot_bat", "/.work_dot_bat/", "  .work_dot_bat/  "} {
		root := t.TempDir()
		write(t, root, ".gitignore", "node_modules/\r\n"+line+"\r\n")
		if !GitignoreCoversState(root) {
			t.Errorf("%q not recognised", line)
		}
	}
	root := t.TempDir()
	if GitignoreCoversState(root) {
		t.Error("no .gitignore")
	}
	write(t, root, ".gitignore", "work_dot_bat_other/\n#.work_dot_bat\n")
	if GitignoreCoversState(root) {
		t.Error("lookalikes")
	}
}
