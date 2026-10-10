package syncer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"work_dot_bat/internal/managed"
)

const (
	fileA = "scripts/work_dot_bat/plan-status.sh"
	fileB = "scripts/work_dot_bat/lib/plan-status.mjs"
)

func vendor() (fstest.MapFS, managed.Manifest) {
	fsys := fstest.MapFS{
		fileA: {Data: []byte("#!/usr/bin/env bash\nexec node lib/plan-status.mjs\n")},
		fileB: {Data: []byte("export const a = 1\n")},
	}
	m := managed.Manifest{Files: map[string]managed.Entry{}}
	for f, d := range fsys {
		m.Files[f] = managed.Entry{Version: "1.1.0", Date: "2026-10-03", SHA256: managed.HashBody(string(d.Data))}
	}
	return fsys, m
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(content), 0o644)
}

func read(root, rel string) string {
	b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	return string(b)
}

func installed(t *testing.T, fsys fstest.MapFS, file, version string) string {
	body, _ := managed.ReadBody(fsys, file)
	text, err := managed.Render(file, version, "2026-01-01", body)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestClassifyEachSituation(t *testing.T) {
	fsys, m := vendor()
	entry := m.Files[fileB]
	cases := []struct {
		name   string
		target *string
		want   State
	}{
		{"missing", nil, Missing},
		{"current", ptr(installed(t, fsys, fileB, "1.1.0")), Current},
		{"outdated", ptr(installed(t, fsys, fileB, "1.0.0")), Outdated},
		{"newer", ptr(installed(t, fsys, fileB, "1.2.0")), Newer},
		{"unmanaged", ptr("// my own file\n"), Unmanaged},
		{"modified", ptr(strings.Replace(installed(t, fsys, fileB, "1.1.0"), "a = 1", "a = 2", 1)), Modified},
	}
	for _, c := range cases {
		if got, _ := Classify(entry, c.target); got != c.want {
			t.Errorf("%s: %s want %s", c.name, got, c.want)
		}
	}
	// Same version but a body that differs from the release is repaired, not left "current".
	other, _ := managed.Render(fileB, "1.1.0", "2026-01-01", "export const a = 99\n")
	if got, _ := Classify(entry, &other); got != Outdated {
		t.Errorf("same-version/different-body: %s", got)
	}
}

func ptr(s string) *string { return &s }

func TestDryRunWritesNothingThenApplyInstallsWithMatchingHeaders(t *testing.T) {
	fsys, m := vendor()
	root := t.TempDir()
	plan := PlanSync(root, m)
	res, err := Apply(root, plan, m, fsys, Options{DryRun: true, Today: "2026-10-03"})
	if err != nil || len(res.Written) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(root, "scripts")); err == nil {
		t.Fatal("a dry run must write nothing")
	}
	if got := FormatReport(plan, res, true); !strings.Contains(got, "Would write: "+fileA+" (new)") || strings.Contains(got, "Changes are in the working tree") {
		t.Errorf("%s", got)
	}

	res, err = Apply(root, plan, m, fsys, Options{Today: "2026-10-03"})
	if err != nil || len(res.Written) != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	for f, e := range m.Files {
		p := managed.Parse(read(root, f))
		if p == nil || p.Version != "1.1.0" || p.Date != "2026-10-03" || p.SHA256 != e.SHA256 || managed.HashBody(p.Body) != p.SHA256 {
			t.Errorf("%s: %+v", f, p)
		}
	}
	if !strings.HasPrefix(read(root, fileA), "#!/usr/bin/env bash\n# work_dot_bat managed file") {
		t.Error("shebang must stay first")
	}
	if got := FormatReport(plan, res, false); !strings.HasSuffix(got, "Changes are in the working tree only; review and commit them yourself.") {
		t.Errorf("%s", got)
	}
	// A second pass is a no-op.
	plan2 := PlanSync(root, m)
	res2, _ := Apply(root, plan2, m, fsys, Options{})
	if len(res2.Written)+len(res2.Skipped)+len(res2.Newer) != 0 || len(plan2.StaleItems()) != 0 {
		t.Errorf("%+v", res2)
	}
	if got := FormatReport(plan2, res2, false); got != "Everything is up to date." {
		t.Errorf("%s", got)
	}
}

func TestLocallyEditedCopyIsNotClobberedWithoutConsent(t *testing.T) {
	fsys, m := vendor()
	root := t.TempDir()
	edited := strings.Replace(installed(t, fsys, fileB, "1.0.0"), "a = 1", "a = LOCAL", 1)
	write(t, root, fileB, edited)
	write(t, root, fileA, "# plain file with no managed header\n")

	plan := PlanSync(root, m)
	res, _ := Apply(root, plan, m, fsys, Options{})
	if len(res.Skipped) != 2 || read(root, fileB) != edited || read(root, fileA) != "# plain file with no managed header\n" {
		t.Fatalf("skipped %v", res.Skipped)
	}
	report := FormatReport(plan, res, false)
	if !strings.Contains(report, "Skipped: "+fileB+" (edited locally) — re-run and confirm to overwrite") ||
		!strings.Contains(report, "Skipped: "+fileA+" (exists without a managed header)") {
		t.Errorf("%s", report)
	}

	// A yes for one file overwrites only that one.
	res, _ = Apply(root, plan, m, fsys, Options{Decide: func(i Item) Decision {
		if i.File == fileB {
			return Overwrite
		}
		return Skip
	}})
	if len(res.Written) != 1 || len(res.Skipped) != 1 || strings.Contains(read(root, fileB), "LOCAL") {
		t.Errorf("%+v", res)
	}
}

func TestOutdatedIsUpdatedAndNewerIsNeverDowngraded(t *testing.T) {
	fsys, m := vendor()
	root := t.TempDir()
	write(t, root, fileB, installed(t, fsys, fileB, "1.0.0"))
	write(t, root, fileA, installed(t, fsys, fileA, "2.0.0"))
	plan := PlanSync(root, m)
	res, _ := Apply(root, plan, m, fsys, Options{})
	if len(res.Written) != 1 || res.Written[0].File != fileB || len(res.Newer) != 1 {
		t.Fatalf("%+v", res)
	}
	if p := managed.Parse(read(root, fileA)); p.Version != "2.0.0" {
		t.Error("a newer copy must be left alone")
	}
	report := FormatReport(plan, res, false)
	if !strings.Contains(report, "Wrote: "+fileB+" (1.0.0 -> 1.1.0)") ||
		!strings.Contains(report, "Warning: "+fileA+" is version 2.0.0, newer than this tool's 1.1.0; left alone (update work_dot_bat).") {
		t.Errorf("%s", report)
	}
}

func TestOrphansAreReportedNotDeleted(t *testing.T) {
	fsys, m := vendor()
	root := t.TempDir()
	orphan := "scripts/work_dot_bat/old-thing.sh"
	text, _ := managed.Render(orphan, "1.0.0", "2026-01-01", "echo old\n")
	write(t, root, orphan, text)
	write(t, root, "scripts/work_dot_bat/mine.sh", "echo not managed\n")
	plan := PlanSync(root, m)
	if len(plan.Orphans) != 1 || plan.Orphans[0] != orphan {
		t.Fatalf("%v", plan.Orphans)
	}
	Apply(root, plan, m, fsys, Options{})
	if read(root, orphan) != text {
		t.Error("orphans must not be deleted")
	}
	if got := FormatReport(plan, Result{}, false); !strings.Contains(got, "Note: "+orphan+" is a managed file this tool no longer ships; delete it if unused.") {
		t.Errorf("%s", got)
	}
}

func TestIsMainCheckout(t *testing.T) {
	root := t.TempDir()
	if IsMainCheckout(root) {
		t.Error("no .git")
	}
	os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere"), 0o644)
	if IsMainCheckout(root) {
		t.Error("a .git file is a linked worktree")
	}
	os.Remove(filepath.Join(root, ".git"))
	os.Mkdir(filepath.Join(root, ".git"), 0o755)
	if !IsMainCheckout(root) {
		t.Error("a .git directory is a main checkout")
	}
}
