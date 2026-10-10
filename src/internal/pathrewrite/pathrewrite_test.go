package pathrewrite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"work_dot_bat/internal/managed"
)

func testMap() []Mapping {
	m := managed.Manifest{Files: map[string]managed.Entry{
		"scripts/work_dot_bat/plan-status.sh":        {},
		"scripts/work_dot_bat/lib/plan-status.mjs":   {},
		"scripts/work_dot_bat/lib/plan-claims.mjs":   {},
		"scripts/work_dot_bat/run-checks.sh":         {},
		"scripts/work_dot_bat/lib/nested/deeper.mjs": {},
	}}
	return BuildPathMap(m)
}

func TestBuildPathMapLongestFirst(t *testing.T) {
	m := testMap()
	has := func(from, to string) bool {
		for _, x := range m {
			if x.From == from && x.To == to {
				return true
			}
		}
		return false
	}
	if !has("scripts/plan-status.sh", "scripts/work_dot_bat/plan-status.sh") || !has("scripts/lib/plan-status.mjs", "scripts/work_dot_bat/lib/plan-status.mjs") {
		t.Errorf("%v", m)
	}
	for i := 1; i < len(m); i++ {
		if len(m[i].From) > len(m[i-1].From) {
			t.Errorf("not longest first: %v", m)
		}
	}
}

func TestRewriteText(t *testing.T) {
	m := testMap()
	in := strings.Join([]string{
		"Run `scripts/plan-status.sh set x Done`.",
		"- run: ./scripts/plan-status.sh check",
		"import { a } from '../scripts/lib/plan-claims.mjs'",
	}, "\n")
	out, n := RewriteText(in, m)
	if n != 3 || !strings.Contains(out, "`scripts/work_dot_bat/plan-status.sh set x Done`") ||
		!strings.Contains(out, "./scripts/work_dot_bat/plan-status.sh check") ||
		!strings.Contains(out, "'../scripts/work_dot_bat/lib/plan-claims.mjs'") {
		t.Errorf("%d\n%s", n, out)
	}
	// Idempotent.
	again, n2 := RewriteText(out, m)
	if n2 != 0 || again != out {
		t.Errorf("second pass changed things: %d", n2)
	}
	// Lookalikes and unvendored scripts are untouched.
	for _, s := range []string{
		"scripts/check-docs.sh scripts/lib/check-docs.mjs",
		"scripts/plan-status.sh.bak",
		"my-scripts/plan-status.sh",
		"scripts/plan-status.shx",
		"x_scripts/plan-status.sh",
	} {
		if _, n := RewriteText(s, m); n != 0 {
			t.Errorf("%q should be left alone", s)
		}
	}
	// A longer name is not a prefix match, but a sentence-ending dot is fine.
	if _, n := RewriteText("scripts/lib/plan-status.mjs and scripts/lib/plan-status.test.ts", m); n != 1 {
		t.Errorf("prefix: %d", n)
	}
	if out, n := RewriteText("See scripts/plan-status.sh.", m); n != 1 || out != "See scripts/work_dot_bat/plan-status.sh." {
		t.Errorf("%q %d", out, n)
	}
	if out, n := RewriteText("scripts/plan-status.sh", nil); out != "scripts/plan-status.sh" || n != 0 {
		t.Error("empty map")
	}
}

func TestIsRewritable(t *testing.T) {
	for _, f := range []string{"agent_docs/plans/old/plan.md", "scripts/work_dot_bat/plan-status.sh", "node_modules/x/index.js", "public/image.png", "LICENSE"} {
		if IsRewritable(f) {
			t.Errorf("%s should be skipped", f)
		}
	}
	for _, f := range []string{"CLAUDE.md", "agent_docs/00_start_here.md", "src/x.ts", ".github/workflows/e2e.yml", "package.json", ".husky/pre-push", ".claude/skills/a/SKILL.md"} {
		if !IsRewritable(f) {
			t.Errorf("%s should be rewritten", f)
		}
	}
}

func TestRewriteRepoPreviewsThenApplies(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
	write("README.md", "use scripts/plan-status.sh and scripts/run-checks.sh\n")
	write("agent_docs/plans/history.md", "scripts/plan-status.sh\n")
	write("scripts/work_dot_bat/x.sh", "scripts/plan-status.sh\n")
	write("data.bin.txt", "scripts/plan-status.sh\x00binary")
	files := []string{"README.md", "agent_docs/plans/history.md", "scripts/work_dot_bat/x.sh", "data.bin.txt", "missing.md"}

	changed, err := RewriteRepo(root, testMap(), false, files)
	if err != nil || len(changed) != 1 || changed[0].File != "README.md" || changed[0].Count != 2 {
		t.Fatalf("%+v %v", changed, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "README.md")); strings.Contains(string(b), "work_dot_bat") {
		t.Error("a preview must not write")
	}
	if _, err := RewriteRepo(root, testMap(), true, files); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(b) != "use scripts/work_dot_bat/plan-status.sh and scripts/work_dot_bat/run-checks.sh\n" {
		t.Errorf("%q", b)
	}
	for _, rel := range []string{"agent_docs/plans/history.md", "scripts/work_dot_bat/x.sh"} {
		if b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); string(b) != "scripts/plan-status.sh\n" {
			t.Errorf("%s was rewritten: %q", rel, b)
		}
	}
}
