package checks

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"work_dot_bat/src/internal/projects"
)

var fixedNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func lines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("const x = 1\n")
	}
	return b.String()
}

func TestCountLines(t *testing.T) {
	for in, want := range map[string]int{"": 0, "\n": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2, "a\n\n": 2} {
		if got := CountLines(in); got != want {
			t.Errorf("%q: %d want %d", in, got, want)
		}
	}
}

func TestClassifyMonolith(t *testing.T) {
	max := 500
	b := MonolithBaseline{MaxLines: &max, Entries: map[string]int{"src/old.ts": 700, "src/shrunk.ts": 900}}
	_, added, grown := ClassifyMonolith(b, []Scanned{
		{"src/ok.ts", 500}, {"src/new.ts", 501}, {"src/old.ts", 700}, {"src/old2.ts", 100},
		{"src/shrunk.ts", 800}, {"src/grew.ts", 600},
	})
	b.Entries["src/grew.ts"] = 550
	_, _, grown2 := ClassifyMonolith(b, []Scanned{{"src/grew.ts", 600}})
	if !reflect.DeepEqual(added, []MonolithViolation{{Path: "src/new.ts", Lines: 501}, {Path: "src/grew.ts", Lines: 600}}) || len(grown) != 0 {
		t.Errorf("added %+v grown %+v", added, grown)
	}
	if !reflect.DeepEqual(grown2, []MonolithViolation{{Path: "src/grew.ts", Lines: 600, BaselineLines: 550}}) {
		t.Errorf("%+v", grown2)
	}
}

func TestListMonolithFilesScopeAndExclusions(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"src/a.ts", "src/b.tsx", "src/c.css", "src/x.test.ts", "src/y.spec.tsx", "src/generated/g.ts", "src/deep/d.ts", "src/readme.md", "other/z.ts"} {
		put(t, root, f, "x")
	}
	if got := ListMonolithFiles(root); !reflect.DeepEqual(got, []string{"src/a.ts", "src/b.tsx", "src/c.css", "src/deep/d.ts"}) {
		t.Errorf("%v", got)
	}
}

func TestListMonolithFilesConfiguredScan(t *testing.T) {
	root := t.TempDir()
	put(t, root, "work_dot_bat.config.json", `{"scanDirs":["ExploringGame","ExploringGame.UnitTests"],"scanExtensions":["*.cs",".fx"],"excludeDirs":["bin","obj"]}`)
	for _, f := range []string{
		"ExploringGame/Shape.cs",
		"ExploringGame/bin/Debug/Generated.cs",
		"ExploringGame/obj/Temp.cs",
		"ExploringGame/Shaders/Effect.fx",
		"ExploringGame/README.md",
		"ExploringGame.UnitTests/TestMaps.cs",
		"src/other.ts",
	} {
		put(t, root, f, "x")
	}
	got := ListMonolithFiles(root)
	want := []string{
		"ExploringGame.UnitTests/TestMaps.cs",
		"ExploringGame/Shaders/Effect.fx",
		"ExploringGame/Shape.cs",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestListTrackedSourceFilesConfiguredScan(t *testing.T) {
	root := gitRepo(t)
	put(t, root, "work_dot_bat.config.json", `{"scanDirs":["ExploringGame","ExploringGame.UnitTests"],"scanExtensions":[".cs",".fx"],"excludeDirs":["bin","obj"]}`)
	for _, f := range []string{
		"ExploringGame/A.cs",
		"ExploringGame/bin/Debug/B.cs",
		"ExploringGame/D.fx",
		"ExploringGame.UnitTests/C.cs",
		"src/E.ts",
	} {
		put(t, root, f, "x")
	}
	gitAdd(t, root)
	if out, err := RealGit(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"); err != nil {
		t.Fatalf("%v %s", err, out)
	}

	got, err := ListTrackedSourceFiles(root, RealGit)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ExploringGame.UnitTests/C.cs",
		"ExploringGame/A.cs",
		"ExploringGame/D.fx",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBreakupPlanPathCSharp(t *testing.T) {
	got := BreakupPlanPath("ExploringGame/GeometryBuilder/Shape.cs", fixedNow)
	want := "plans/october_2026/monolith_breakup/exploringgame_geometrybuilder_shape.md"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}


func TestMonolithCheckWritesAPlanAndRatchets(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "agent_docs", "plans"), 0o755)
	put(t, root, "src/big.ts", lines(501))
	put(t, root, "src/fine.ts", lines(10))
	var out, errOut strings.Builder
	if code := MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow); code != 1 {
		t.Fatalf("exit %d %s", code, errOut.String())
	}
	for _, want := range []string{
		"Monolith check found 1 file(s) exceeding 500 lines:\n", "  + NEW  src/big.ts (501 lines)\n",
		"    -> wrote plan: agent_docs/plans/october_2026/monolith_breakup/src_big.md\n",
		"`work_dot_bat check-monolith baseline --accept <path> --reason \"<why>\"`.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	plan, _ := os.ReadFile(filepath.Join(root, "agent_docs/plans/october_2026/monolith_breakup/src_big.md"))
	if !strings.HasPrefix(string(plan), "---\nstatus: Not Started\n---\n") || !strings.Contains(string(plan), "`src/big.ts` is 501 lines") {
		t.Errorf("plan:\n%s", plan)
	}

	// Running again finds the plan already tracked.
	out.Reset()
	MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow)
	if !strings.Contains(out.String(), "-> already tracked plan:") {
		t.Errorf("%s", out.String())
	}

	// Accepting it with a reason records the file; the check then passes at that size, fails if it grows.
	out.Reset()
	if code := MonolithMain(root, []string{"baseline", "--accept", "src/big.ts", "--reason", "legacy parser"}, &out, &errOut, fixedNow); code != 0 {
		t.Fatalf("%d %s %s", code, out.String(), errOut.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs/monolith_baseline.json"))
	if string(b) != "{\n  \"maxLines\": 500,\n  \"entries\": {\n    \"src/big.ts\": 501\n  },\n  \"reasons\": {\n    \"src/big.ts\": \"legacy parser\"\n  }\n}\n" {
		t.Errorf("%s", b)
	}
	out.Reset()
	if code := MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow); code != 0 || !strings.Contains(out.String(), "Monolith check passed (500-line threshold).") {
		t.Errorf("%d %s", code, out.String())
	}
	put(t, root, "src/big.ts", lines(520))
	out.Reset()
	if code := MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow); code != 1 || !strings.Contains(out.String(), "  ^ GREW src/big.ts (520 lines, baseline was 501)") {
		t.Errorf("%d %s", code, out.String())
	}
	// Shrinking tightens the baseline (unless told not to).
	put(t, root, "src/big.ts", lines(700))
	MonolithMain(root, []string{"baseline", "--accept", "src/big.ts", "--reason", "legacy parser"}, &out, &errOut, fixedNow)
	before, _ := os.ReadFile(filepath.Join(root, "agent_docs/monolith_baseline.json"))
	put(t, root, "src/big.ts", lines(600))
	out.Reset()
	MonolithMain(root, []string{"check", "--no-tighten"}, &out, &errOut, fixedNow)
	if after, _ := os.ReadFile(filepath.Join(root, "agent_docs/monolith_baseline.json")); string(after) != string(before) {
		t.Error("--no-tighten must not rewrite the baseline")
	}
	out.Reset()
	MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow)
	if !strings.Contains(out.String(), "Monolith check passed and baseline tightened.") {
		t.Errorf("%s", out.String())
	}
	if after, _ := os.ReadFile(filepath.Join(root, "agent_docs/monolith_baseline.json")); !strings.Contains(string(after), `"src/big.ts": 600`) {
		t.Errorf("%s", after)
	}
	// Dropping under the threshold removes the entry and its reason.
	put(t, root, "src/big.ts", lines(10))
	MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow)
	if after, _ := os.ReadFile(filepath.Join(root, "agent_docs/monolith_baseline.json")); string(after) != "{\n  \"maxLines\": 500,\n  \"entries\": {}\n}\n" {
		t.Errorf("%s", after)
	}
}

func TestMonolithBaselineRefusesToGrowAndNeedsAReason(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "agent_docs"), 0o755)
	put(t, root, "src/big.ts", lines(501))
	var out, errOut strings.Builder
	if code := MonolithMain(root, []string{"baseline"}, &out, &errOut, fixedNow); code != 1 ||
		!strings.Contains(out.String(), "Baseline NOT updated — it only shrinks by default. Refused 1 file(s) over 500 lines:") ||
		!strings.Contains(out.String(), "  src/big.ts (501 lines, not in the baseline)") {
		t.Errorf("%d %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "agent_docs/monolith_baseline.json")); err == nil {
		t.Error("a refused baseline must write nothing")
	}
	errOut.Reset()
	if code := MonolithMain(root, []string{"baseline", "--accept", "src/big.ts"}, &out, &errOut, fixedNow); code != 1 || !strings.Contains(errOut.String(), `--accept needs a --reason "<text>"`) {
		t.Errorf("%d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := MonolithMain(root, []string{"baseline", "--accept", "src/nope.ts", "--reason", "x"}, &out, &errOut, fixedNow); code != 1 || !strings.Contains(errOut.String(), "that file is not over the 500-line threshold") {
		t.Errorf("%d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := MonolithMain(root, []string{"nope"}, &out, &errOut, fixedNow); code != 1 || !strings.Contains(errOut.String(), `Unknown command: "nope"`) {
		t.Errorf("%d %s", code, errOut.String())
	}
}

func TestMonolithReopensAFinishedPlanWhenTheFileRegrows(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "agent_docs", "plans"), 0o755)
	put(t, root, "src/big.ts", lines(501))
	var out, errOut strings.Builder
	MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow)
	planRel := "agent_docs/plans/october_2026/monolith_breakup/src_big.md"
	plan, _ := os.ReadFile(filepath.Join(root, planRel))
	os.WriteFile(filepath.Join(root, planRel), []byte(strings.Replace(string(plan), "status: Not Started", "status: Done", 1)), 0o644)
	out.Reset()
	MonolithMain(root, []string{"check"}, &out, &errOut, fixedNow)
	if !strings.Contains(out.String(), "-> reopened (file regrew) plan:") {
		t.Errorf("%s", out.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, planRel))
	if !strings.Contains(string(after), "status: Not Started") || !strings.Contains(string(after), "## Regrowth — 2026-10-03: 501 lines") {
		t.Errorf("%s", after)
	}
}

func TestDuplicationHelpers(t *testing.T) {
	if got := NormalizeLines("  a  \n\n// c\n * d\n/* e */\r\n b\r\n"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("%q", got)
	}
	if got := ExtractWindows([]string{"1", "2", "3", "4"}, 2, "\n"); !reflect.DeepEqual(got, []string{"1\n2", "2\n3", "3\n4"}) {
		t.Errorf("%q", got)
	}
	if len(ExtractWindows([]string{"1"}, 2, "\n")) != 0 {
		t.Error("too short")
	}
	for in, want := range map[string]bool{"src/a.ts": true, "src/a.tsx": true, "src/a.test.ts": false, "src/generated/a.ts": false, "src/a.js": false} {
		if IsTrackedSourceFile(in) != want {
			t.Errorf("%s", in)
		}
	}
	dups := FindDuplicates([]FileWindows{
		{"b.ts", []string{"w1", "w2"}}, {"a.ts", []string{"w1", "w3"}}, {"c.ts", []string{"w1", "w2"}},
	})
	if len(dups) != 2 || dups[0].Window != "w1" || !reflect.DeepEqual(dups[0].Files, []string{"a.ts", "b.ts", "c.ts"}) || !reflect.DeepEqual(dups[1].Files, []string{"b.ts", "c.ts"}) {
		t.Errorf("%+v", dups)
	}
	pairs := TopFilePairs(dups, 10)
	if len(pairs) != 3 || pairs[0].Pair != "b.ts <-> c.ts" || pairs[0].Count != 2 || pairs[1].Pair != "a.ts <-> b.ts" {
		t.Errorf("%+v", pairs)
	}
	if got := TopFilePairs(dups, 1); len(got) != 1 {
		t.Errorf("limit: %+v", got)
	}
}

func dupBlock(tag string) string {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		b.WriteString("shared line " + string(rune('a'+i)) + " " + tag + "\n")
	}
	return b.String()
}

func TestDuplicationCheckRatchet(t *testing.T) {
	root := gitRepo(t)
	os.MkdirAll(filepath.Join(root, "agent_docs"), 0o755)
	shared := dupBlock("same")
	put(t, root, "src/a.ts", "// header\n"+shared+"unique a\n")
	put(t, root, "src/b.ts", shared+"unique b\n")
	put(t, root, "src/gone.ts", shared)
	put(t, root, "src/skip.test.ts", shared)
	gitAdd(t, root)
	if out, err := RealGit(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	os.Remove(filepath.Join(root, "src/gone.ts")) // listed by HEAD, gone from the working tree
	var out, errOut strings.Builder

	// Baseline 0: one shared 8-line block across a.ts and b.ts fails.
	if code := DuplicationMain(root, []string{"check"}, &out, &errOut, RealGit); code != 1 {
		t.Fatalf("exit %d %s %s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "Duplication check failed: 1 duplicated 8-line blocks (baseline 0).") ||
		!strings.Contains(out.String(), "  1x  src/a.ts <-> src/b.ts") {
		t.Errorf("%s", out.String())
	}
	// Accepting records it; regressions need the flag.
	out.Reset()
	if code := DuplicationMain(root, []string{"update"}, &out, &errOut, RealGit); code != 1 || !strings.Contains(errOut.String(), "Refusing to raise the duplication baseline from 0 to 1 without --allow-regression.") {
		t.Errorf("%d %s", code, errOut.String())
	}
	out.Reset()
	if code := DuplicationMain(root, []string{"update", "--allow-regression"}, &out, &errOut, RealGit); code != 0 || out.String() != "Baseline updated: 1 duplicated block(s) recorded.\n" {
		t.Errorf("%d %s", code, out.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, "agent_docs/duplication_baseline.json"))
	if !strings.HasPrefix(string(b), "{\n  \"totalDuplicatedBlocks\": 1,\n  \"note\": \"See scripts/work_dot_bat/lib/check-duplication.mjs") {
		t.Errorf("%s", b)
	}
	out.Reset()
	if code := DuplicationMain(root, []string{"check"}, &out, &errOut, RealGit); code != 0 || out.String() != "Duplication check passed (1 <= baseline 1).\n" {
		t.Errorf("%d %s", code, out.String())
	}
	// Removing the duplication tightens the baseline (not with --no-tighten).
	put(t, root, "src/b.ts", "different\n")
	out.Reset()
	DuplicationMain(root, []string{"check", "--no-tighten"}, &out, &errOut, RealGit)
	if out.String() != "Duplication check passed (0 <= baseline 1).\n" {
		t.Errorf("%s", out.String())
	}
	out.Reset()
	DuplicationMain(root, []string{"check"}, &out, &errOut, RealGit)
	if out.String() != "Duplication check passed and baseline tightened (1 -> 0).\n" {
		t.Errorf("%s", out.String())
	}
}

func TestSkillsCheck(t *testing.T) {
	root := t.TempDir()
	skill := func(tree, dir, body string) { put(t, root, tree+"/"+dir+"/SKILL.md", body) }
	put(t, root, "agent_docs/instructions/a.md", "x")
	put(t, root, "agent_docs/instructions/b.md", "x")
	skill(".agents/skills", "do-work", "---\nname: do-work\ndescription: does work\n---\nSee agent_docs/instructions/a.md\n")
	skill(".claude/skills", "do-work", "---\nname: do_work\ndescription: does work\n---\nSee `agent_docs\\instructions\\b.md`\n")
	skill(".claude/skills", "nameless", "---\ndescription: d\n---\n")
	skill(".claude/skills", "wrongname", "---\nname: other\ndescription: d\n---\n")
	skill(".claude/skills", "nodesc", "---\nname: nodesc\n---\nagent_docs/instructions/missing.md\n")
	r := CheckSkills(root)
	if r.Passed || r.AgentsCount != 1 || r.ClaudeCount != 4 {
		t.Fatalf("%+v", r)
	}
	var got []string
	for _, v := range r.Violations {
		got = append(got, v.File+": "+v.Reason)
	}
	want := []string{
		`.claude/skills/nameless/SKILL.md: Missing "name:" field in frontmatter`,
		`.claude/skills/nodesc/SKILL.md: Missing or empty "description:" field in frontmatter`,
		`.claude/skills/nodesc/SKILL.md: Referenced file does not exist: agent_docs/instructions/missing.md`,
		`.claude/skills/wrongname/SKILL.md: Frontmatter name "other" does not match directory name "wrongname"`,
		`.claude/skills/do-work/SKILL.md: Cross-tree routing mismatch for skill "do-work": .agents points to "agent_docs/instructions/a.md", but .claude points to "agent_docs/instructions/b.md"`,
	}
	sortStrings := func(s []string) []string { c := append([]string{}, s...); sortInPlace(c); return c }
	if !reflect.DeepEqual(sortStrings(got), sortStrings(want)) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	clean := t.TempDir()
	var out, errOut strings.Builder
	if code := SkillsMain(clean, nil, &out, &errOut); code != 0 || out.String() != "Skill tree integrity check passed (0 .agents, 0 .claude skills scanned).\n" {
		t.Errorf("%d %s", code, out.String())
	}
	out.Reset()
	if code := SkillsMain(root, nil, &out, &errOut); code != 1 || !strings.HasPrefix(out.String(), "Skill tree integrity check found 5 violation(s):\n  ❌ ") {
		t.Errorf("%d %s", code, out.String())
	}
}

func sortInPlace(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestEvaluateAudit(t *testing.T) {
	ok, v, err := EvaluateAudit([]byte(`{"metadata":{"vulnerabilities":{"low":3,"moderate":2,"high":0,"critical":0}}}`))
	if err != nil || !ok || len(v) != 0 {
		t.Errorf("%v %v %v", ok, v, err)
	}
	ok, v, _ = EvaluateAudit([]byte(`{"metadata":{"vulnerabilities":{"high":1,"critical":2}}}`))
	if ok || len(v) != 2 || !strings.HasPrefix(v[0].Reason, "1 high severity vulnerability found (run `npm audit`") || !strings.HasPrefix(v[1].Reason, "2 critical severity vulnerabilities found") {
		t.Errorf("%v", v)
	}
	if ok, _, _ := EvaluateAudit([]byte(`{}`)); !ok {
		t.Error("an empty report passes")
	}
	if _, _, err := EvaluateAudit([]byte(`not json`)); err == nil {
		t.Error("garbage is an error")
	}
}

func TestBuildCheckListOrderAndPhases(t *testing.T) {
	e := Env{Out: &strings.Builder{}, Err: &strings.Builder{}}
	cfg := Config{
		ExtraChecks: []ExtraCheck{{Name: "Docs", Command: Command{Cmd: "node", Args: []string{"docs.js"}}}},
		Build:       []Command{{Cmd: "npm", Args: []string{"run", "lint"}}},
		LightTest:   &Command{Cmd: "npm", Args: []string{"test"}},
		HeavyTest:   &Command{Cmd: "npm", Args: []string{"run", "e2e"}},
	}
	var names, phases []string
	for _, c := range e.BuildCheckList(cfg, false) {
		names = append(names, c.Name)
		phases = append(phases, c.Phase)
	}
	wantNames := []string{"Plan Drift Check", "Monolith Check", "Duplication Check", "Structural Duplication Check", "Source Assertion Check",
		"Skill Tree Integrity Check", "npm audit Gate", "Docs", "Build: npm run lint", "Light tests: npm test", "Heavy tests: npm run e2e"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Errorf("%v", names)
	}
	if phases[len(phases)-2] != "unit" || phases[len(phases)-1] != "e2e" || phases[0] != "" {
		t.Errorf("%v", phases)
	}
	only := Config{Checks: []string{"skills", "monolith"}, ChecksSet: true}
	var got []string
	for _, c := range e.BuildCheckList(only, false) {
		got = append(got, c.Name)
	}
	if !reflect.DeepEqual(got, []string{"Monolith Check", "Skill Tree Integrity Check"}) {
		t.Errorf("tool checks always run in the fixed order: %v", got)
	}
	if n := len(e.BuildCheckList(Config{Checks: []string{}, ChecksSet: true}, false)); n != 0 {
		t.Errorf("an empty checks list disables them all: %d", n)
	}
}

func TestRunChecksGoldenAndSkippingTests(t *testing.T) {
	var out strings.Builder
	var ran []string
	e := Env{Out: &out, Err: &out, Run: func(_ context.Context, c Check, cwd string) int {
		ran = append(ran, c.Name+"@"+cwd)
		if strings.HasPrefix(c.Name, "Build") {
			return 1
		}
		return 0
	}}
	checks := []Check{
		{Name: "Native OK", Native: func(context.Context, string) int { return 0 }},
		{Name: "Build: bad", Cmd: "x"},
		{Name: "Build: also ran", Cmd: "y"},
		{Name: "Light tests: t", Cmd: "t", Phase: "unit"},
		{Name: "Heavy tests: t", Cmd: "h", Phase: "e2e"},
	}
	failures, skipped := e.RunChecks(context.Background(), checks, "/work")
	if !reflect.DeepEqual(failures, []string{"Build: bad"}) && !reflect.DeepEqual(failures, []string{"Build: bad", "Build: also ran"}) {
		t.Errorf("%v", failures)
	}
	if len(failures) != 2 || !reflect.DeepEqual(skipped, []string{"Light tests: t", "Heavy tests: t"}) {
		t.Errorf("failures %v skipped %v", failures, skipped)
	}
	if !reflect.DeepEqual(ran, []string{"Build: bad@/work", "Build: also ran@/work"}) {
		t.Errorf("every mechanical check runs, the tests do not: %v", ran)
	}
	want := "\n--- Native OK ---\n✅ Native OK passed.\n" +
		"\n--- Build: bad ---\n❌ Build: bad failed.\n" +
		"\n--- Build: also ran ---\n❌ Build: also ran failed.\n" +
		"\n--- Light tests: t ---\n⏭️  Skipped — 2 earlier check(s) failed: Build: bad, Build: also ran\n" +
		"\n--- Heavy tests: t ---\n⏭️  Skipped — 2 earlier check(s) failed: Build: bad, Build: also ran\n" +
		"\n===================================================\n" +
		"❌ 2 check(s) failed:\n   - Build: bad\n   - Build: also ran\n" +
		"⏭️  2 check(s) skipped (an earlier check failed):\n   - Light tests: t\n   - Heavy tests: t\n" +
		"===================================================\n\n"
	if out.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", out.String(), want)
	}

	out.Reset()
	f2, s2 := e.RunChecks(context.Background(), []Check{{Name: "ok", Native: func(context.Context, string) int { return 0 }}}, "/w")
	if len(f2)+len(s2) != 0 || out.String() != "\n--- ok ---\n✅ ok passed.\n\n===================================================\n✅ All automated tests passed!\n===================================================\n\n" {
		t.Errorf("%q", out.String())
	}
}

func TestRunChecksMainUsageAndConfigErrors(t *testing.T) {
	var out, errOut strings.Builder
	e := Env{Out: &out, Err: &errOut}
	if code := e.RunChecksMain(context.Background(), []string{"--bogus"}); code != 2 || !strings.Contains(errOut.String(), "Unknown argument: --bogus") {
		t.Errorf("%d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := e.RunChecksMain(context.Background(), []string{"--cwd", t.TempDir()}); code != 2 || !strings.Contains(errOut.String(), "No work_dot_bat.config.json in ") {
		t.Errorf("%d %s", code, errOut.String())
	}
	root := t.TempDir()
	put(t, root, "work_dot_bat.config.json", `{"checks":[],"build":[{"cmd":"x","args":[]}],"lightTest":{"cmd":"t","args":[]}}`)
	var ran []string
	e.Run = func(_ context.Context, c Check, cwd string) int { ran = append(ran, c.Name); return 0 }
	if code := e.RunChecksMain(context.Background(), []string{"--cwd", root, "--skip-tests"}); code != 0 || !reflect.DeepEqual(ran, []string{"Build: x"}) {
		t.Errorf("%d %v", code, ran)
	}
	ran = nil
	e.Run = func(_ context.Context, c Check, cwd string) int { ran = append(ran, c.Name); return 1 }
	if code := e.RunChecksMain(context.Background(), []string{"--cwd", root}); code != 1 || len(ran) != 1 {
		t.Errorf("%d %v", code, ran)
	}

	// Test RunChecksMain with config in projects.json and NO work_dot_bat.config.json
	regDir := t.TempDir()
	regFile := filepath.Join(regDir, "projects.json")
	pRoot := t.TempDir()
	regContent := `{"projects":[{"name":"regproj","path":"` + filepath.ToSlash(pRoot) + `","build":[{"cmd":"regbuild","args":[]}],"checks":[]}]}`
	os.WriteFile(regFile, []byte(regContent), 0o644)
	t.Setenv(projects.RegistryEnv, regFile)
	ran = nil
	e.Run = func(_ context.Context, c Check, cwd string) int { ran = append(ran, c.Name); return 0 }
	if code := e.RunChecksMain(context.Background(), []string{"--cwd", pRoot, "--skip-tests"}); code != 0 || !reflect.DeepEqual(ran, []string{"Build: regbuild"}) {
		t.Errorf("%d %v", code, ran)
	}
}

func TestNodeBackedChecksNeedTheSyncedScript(t *testing.T) {
	var out, errOut strings.Builder
	e := Env{Out: &out, Err: &errOut}
	root := t.TempDir()
	cfg := Config{Checks: []string{"source-assertions"}, ChecksSet: true}
	checks := e.BuildCheckList(cfg, false)
	if len(checks) != 1 || checks[0].Native(context.Background(), root) != 1 || !strings.Contains(errOut.String(), "check-source-assertions not found in") {
		t.Errorf("%v %s", checks, errOut.String())
	}
	// With the script present the node command is what runs (--no-tighten reaches the ratchet one only).
	put(t, root, "scripts/work_dot_bat/typescript/check-structural-duplication.mjs", "")
	var got []Check
	e.Run = func(_ context.Context, c Check, cwd string) int { got = append(got, c); return 0 }
	cfg = Config{Checks: []string{"structural-duplication"}, ChecksSet: true}
	e.BuildCheckList(cfg, true)[0].Native(context.Background(), root)
	if len(got) != 1 || got[0].Cmd != "node" || !reflect.DeepEqual(got[0].Args[1:], []string{"check", "--no-tighten"}) {
		t.Errorf("%+v", got)
	}
}

func TestRunAutomatedTestsWithProjectsTestsConfig(t *testing.T) {
	root := t.TempDir()
	put(t, root, "work_dot_bat.config.json", `{"checks":[]}`)

	proj := projects.Project{
		Name: "myproj",
		Path: root,
		Tests: projects.TestsConfig{
			UltraLight: projects.CommandList{{"echo", "ultra_light"}},
			Light:      projects.CommandList{{"echo", "light"}},
			Heavy:      projects.CommandList{{"echo", "heavy"}},
			UltraHeavy: projects.CommandList{{"echo", "ultra_heavy"}},
		},
	}

	var ran []string
	e := Env{
		Out:      os.Stdout,
		Err:      os.Stderr,
		Projects: []projects.Project{proj},
		Run: func(_ context.Context, c Check, cwd string) int {
			ran = append(ran, c.Name)
			return 0
		},
	}

	// 1. Default / all tiers
	ran = nil
	failures := e.RunAutomatedTests(context.Background(), root, root)
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	wantAll := []string{"[ultra_light] echo ultra_light", "[light] echo light", "[heavy] echo heavy", "[ultra_heavy] echo ultra_heavy"}
	if !reflect.DeepEqual(ran, wantAll) {
		t.Errorf("got %v, want %v", ran, wantAll)
	}

	// 2. Target: "light" -> runs ultra_light then light
	ran = nil
	failures = e.RunAutomatedTests(context.Background(), root, root, "light")
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	wantLight := []string{"[ultra_light] echo ultra_light", "[light] echo light"}
	if !reflect.DeepEqual(ran, wantLight) {
		t.Errorf("got %v, want %v", ran, wantLight)
	}

	// 3. Target: "heavy" -> runs ultra_light, light, heavy
	ran = nil
	failures = e.RunAutomatedTests(context.Background(), root, root, "heavy")
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	wantHeavy := []string{"[ultra_light] echo ultra_light", "[light] echo light", "[heavy] echo heavy"}
	if !reflect.DeepEqual(ran, wantHeavy) {
		t.Errorf("got %v, want %v", ran, wantHeavy)
	}

	// 4. Target: "ultra_heavy" -> runs all 4
	ran = nil
	failures = e.RunAutomatedTests(context.Background(), root, root, "ultra_heavy")
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	if !reflect.DeepEqual(ran, wantAll) {
		t.Errorf("got %v, want %v", ran, wantAll)
	}
}

func TestRunAutomatedTestsFallbackToWorkDotBatConfig(t *testing.T) {
	root := t.TempDir()
	put(t, root, "work_dot_bat.config.json", `{
		"checks":[],
		"lightTest": {"cmd": "go", "args": ["test", "./..."]},
		"heavyTest": {"cmd": "go", "args": ["test", "-tags=e2e", "./..."]}
	}`)

	// Project has no TestsConfig configured
	proj := projects.Project{
		Name: "legacyproj",
		Path: root,
	}

	var ran []string
	e := Env{
		Out:      os.Stdout,
		Err:      os.Stderr,
		Projects: []projects.Project{proj},
		Run: func(_ context.Context, c Check, cwd string) int {
			ran = append(ran, c.Name)
			return 0
		},
	}

	// 1. Default (no layer specified) -> runs both lightTest and heavyTest
	ran = nil
	failures := e.RunAutomatedTests(context.Background(), root, root)
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	wantLegacyAll := []string{"Light tests: go test ./...", "Heavy tests: go test -tags=e2e ./..."}
	if !reflect.DeepEqual(ran, wantLegacyAll) {
		t.Errorf("got %v, want %v", ran, wantLegacyAll)
	}

	// 2. Target: "light" -> runs only lightTest
	ran = nil
	failures = e.RunAutomatedTests(context.Background(), root, root, "light")
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	wantLegacyLight := []string{"Light tests: go test ./..."}
	if !reflect.DeepEqual(ran, wantLegacyLight) {
		t.Errorf("got %v, want %v", ran, wantLegacyLight)
	}

	// 3. Target: "heavy" -> runs lightTest and heavyTest
	ran = nil
	failures = e.RunAutomatedTests(context.Background(), root, root, "heavy")
	if len(failures) != 0 {
		t.Fatalf("expected 0 failures, got %v", failures)
	}
	if !reflect.DeepEqual(ran, wantLegacyAll) {
		t.Errorf("got %v, want %v", ran, wantLegacyAll)
	}
}
