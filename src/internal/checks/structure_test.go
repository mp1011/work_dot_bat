package checks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"work_dot_bat/src/internal/projects"
)

func TestParseHeadingPath(t *testing.T) {
	for in, want := range map[string]string{
		"## Top level":                      "",
		"### src\\audio":                    "src/audio",
		"#### src\\pages\\audio_processing": "src/pages/audio_processing",
		"## src":                            "src",
	} {
		if got, ok := ParseHeadingPath(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"- `App.tsx` - shell", "# PROJECT STRUCTURE"} {
		if _, ok := ParseHeadingPath(in); ok {
			t.Errorf("%q is not a section heading", in)
		}
	}
}

func TestParseBullet(t *testing.T) {
	if b := ParseBullet("- `App.tsx` - App shell."); b == nil || b.Indent != 0 || !reflect.DeepEqual(b.Tokens, []string{"App.tsx"}) || !b.HasSummary {
		t.Errorf("%+v", b)
	}
	if b := ParseBullet("- `perfSampler.ts` - `createPerfSampler()` factory: `performance.memory`."); b == nil || !reflect.DeepEqual(b.Tokens, []string{"perfSampler.ts"}) {
		t.Errorf("backticks inside the summary are not tokens: %+v", b)
	}
	if b := ParseBullet("- `a.html`, `b.html`, `c.html` - Topic texts."); b == nil || !reflect.DeepEqual(b.Tokens, []string{"a.html", "b.html", "c.html"}) {
		t.Errorf("%+v", b)
	}
	if b := ParseBullet("  - `deep_dives\\`"); b == nil || b.Indent != 2 || !reflect.DeepEqual(b.Tokens, []string{"deep_dives\\"}) || b.HasSummary {
		t.Errorf("%+v", b)
	}
	if ParseBullet("plain prose") != nil || ParseBullet("- no backtick") != nil {
		t.Error("not a bullet")
	}
}

var srcAudioRegion = []string{
	"## src",
	"",
	"- `App.tsx` - App shell.",
	"",
	"### src\\audio",
	"",
	"- `audioService.ts` - Audio service.",
	"- `audioSignal.ts` - Signal state machine.",
}

func dropPaths(d []Dropped) []string {
	var out []string
	for _, x := range d {
		out = append(out, x.Path)
	}
	return out
}

func TestReconcilePreservesExistingSummaries(t *testing.T) {
	disk := []string{"src/App.tsx", "src/audio/audioService.ts", "src/audio/audioSignal.ts"}
	lines, added, dropped := ReconcileRegion(srcAudioRegion, disk)
	if len(added) != 0 || len(dropped) != 0 || !reflect.DeepEqual(lines, srcAudioRegion) {
		t.Errorf("%v %v %v", lines, added, dropped)
	}
}

func TestReconcileInsertsOneTodoForANewFile(t *testing.T) {
	disk := []string{"src/App.tsx", "src/audio/audioService.ts", "src/audio/audioSignal.ts", "src/audio/newThing.ts"}
	lines, added, dropped := ReconcileRegion(srcAudioRegion, disk)
	want := append(append([]string{}, srcAudioRegion...), "- `newThing.ts` - "+TodoSummary)
	if !reflect.DeepEqual(added, []string{"src/audio/newThing.ts"}) || len(dropped) != 0 || !reflect.DeepEqual(lines, want) {
		t.Errorf("%v %v", lines, added)
	}
	// Idempotent.
	again, a2, d2 := ReconcileRegion(lines, disk)
	if !reflect.DeepEqual(again, lines) || len(a2) != 0 || len(d2) != 0 {
		t.Errorf("second pass changed things: %v", again)
	}
}

func TestReconcileDropsADeletedFilesBullet(t *testing.T) {
	lines, added, dropped := ReconcileRegion(srcAudioRegion, []string{"src/App.tsx", "src/audio/audioService.ts"})
	if len(added) != 0 || !reflect.DeepEqual(dropPaths(dropped), []string{"src/audio/audioSignal.ts"}) {
		t.Errorf("%v %v", added, dropped)
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "audioSignal") || !strings.Contains(joined, "- `audioService.ts` - Audio service.") {
		t.Errorf("%v", lines)
	}
}

func TestReconcileExpandsALeafSummaryDirectory(t *testing.T) {
	region := []string{"## Top level", "", "- `agent_docs\\`", "  - `plans\\`", "    - `old\\` - Archive of historical planning docs."}
	lines, added, _ := ReconcileRegion(region, []string{"agent_docs/plans/old/ancient.md"})
	joined := "\n" + strings.Join(lines, "\n") + "\n"
	if !reflect.DeepEqual(added, []string{"agent_docs/plans/old/ancient.md"}) ||
		!strings.Contains(joined, "\n    - `old\\` - Archive of historical planning docs.\n") ||
		!strings.Contains(joined, "\n      - `ancient.md` - "+TodoSummary+"\n") {
		t.Errorf("%v %v", lines, added)
	}
}

func TestReconcileSynthesisesASectionForAnUnlistedDirectory(t *testing.T) {
	disk := []string{"src/App.tsx", "src/audio/audioService.ts", "src/audio/audioSignal.ts", "src/noise_page/noisePageBands.ts"}
	lines, added, _ := ReconcileRegion(srcAudioRegion, disk)
	joined := "\n" + strings.Join(lines, "\n") + "\n"
	if !reflect.DeepEqual(added, []string{"src/noise_page/noisePageBands.ts"}) ||
		!strings.Contains(joined, "\n- `noise_page\\`\n") || !strings.Contains(joined, "\n  - `noisePageBands.ts` - "+TodoSummary+"\n") {
		t.Errorf("%v", lines)
	}
}

func TestReconcileIsStableWhenSubdirAndParentEndOnTheSameLine(t *testing.T) {
	region := []string{
		"## Top level", "", "- `agent_docs\\`", "  - `plans\\`", "    - `voice_leading_phase3\\`",
		"      - `playback_options_plan.md` - Existing plan.", "  - `project_health\\`", "    - `health_1.md` - Existing health note.",
	}
	disk := []string{
		"agent_docs/plans/voice_leading_phase3/playback_options_plan.md",
		"agent_docs/plans/voice_leading_phase3/task1.md",
		"agent_docs/plans/done/note.md",
		"agent_docs/project_health/health_1.md",
	}
	first, _, _ := ReconcileRegion(region, disk)
	second, added, dropped := ReconcileRegion(first, disk)
	if len(added) != 0 || len(dropped) != 0 || !reflect.DeepEqual(second, first) {
		t.Errorf("not idempotent:\n%s\n---\n%s", strings.Join(first, "\n"), strings.Join(second, "\n"))
	}
}

func TestReconcileCarriesASummaryAcrossAMovedFile(t *testing.T) {
	region := []string{"## Top level", "", "- `agent_docs\\`", "  - `instructions\\`", "    - `documentation-sync.md` - Guide for tier-by-tier doc review."}
	lines, added, dropped := ReconcileRegion(region, []string{"agent_docs/instructions/high_trust/documentation-sync.md"})
	if !reflect.DeepEqual(dropPaths(dropped), []string{"agent_docs/instructions/documentation-sync.md"}) ||
		!reflect.DeepEqual(added, []string{"agent_docs/instructions/high_trust/documentation-sync.md"}) {
		t.Errorf("%v %v", added, dropped)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "- `documentation-sync.md` - Guide for tier-by-tier doc review.") || strings.Contains(joined, TodoSummary) {
		t.Errorf("%v", lines)
	}
}

func TestReconcileGroupBulletsConsumeEveryNamedFile(t *testing.T) {
	region := []string{"## src", "", "### src\\navigation", "", "- `learning_content\\`", "  - `a.html`, `b.html` - Topic texts."}
	_, added, dropped := ReconcileRegion(region, []string{"src/navigation/learning_content/a.html", "src/navigation/learning_content/b.html"})
	if len(added) != 0 || len(dropped) != 0 {
		t.Errorf("%v %v", added, dropped)
	}
}

func TestReconcileResolvesAMisfiledBullet(t *testing.T) {
	region := []string{"## src", "", "### src\\diagnostics", "", "- `types.ts` - sample shapes.", "- `index.css` - Minimal global style resets."}
	lines, _, dropped := ReconcileRegion(region, []string{"src/diagnostics/types.ts", "src/index.css"})
	if len(dropped) != 0 || !strings.Contains(strings.Join(lines, "\n"), "- `index.css` - Minimal global style resets.") {
		t.Errorf("%v %v", lines, dropped)
	}
}

const doc = "# PROJECT STRUCTURE\n\nIntro prose.\n\n## src\n\n### src\\audio\n\n- `audioService.ts` - Audio service.\n\n## Quick mental model\n\n- closing prose."

func TestRegenerateDocInsertsMarkersAndIsStable(t *testing.T) {
	disk := []string{"src/audio/audioService.ts"}
	first, added, dropped, err := RegenerateDoc(doc, disk)
	if err != nil || len(added) != 0 || len(dropped) != 0 {
		t.Fatalf("%v %v %v", err, added, dropped)
	}
	begin, end := strings.Index(first, BeginMarker), strings.Index(first, EndMarker)
	if begin < 0 || end < begin || !strings.Contains(first[:begin], "Intro prose.") || !strings.Contains(first[end:], "closing prose.") ||
		!strings.Contains(first[begin:end], "- `audioService.ts` - Audio service.") {
		t.Errorf("%s", first)
	}
	second, _, _, _ := RegenerateDoc(first, disk)
	if second != first {
		t.Errorf("a second run changed the doc:\n%s", second)
	}
	// CRLF documents keep their line endings.
	crlf, _, _, _ := RegenerateDoc(strings.ReplaceAll(doc, "\n", "\r\n"), disk)
	if strings.Contains(strings.ReplaceAll(crlf, "\r\n", ""), "\n") {
		t.Error("CRLF must be preserved throughout")
	}
	if _, _, _, err := RegenerateDoc("# no region here\n", disk); err == nil {
		t.Error("expected a region error")
	}
}

func TestFindUnsummarizedFiles(t *testing.T) {
	with := func(body ...string) string {
		return strings.Join(append(append([]string{"# PROJECT STRUCTURE", "", BeginMarker, "## src", "", "### src\\audio", ""}, body...), EndMarker, "", "## Quick mental model"), "\n")
	}
	got := FindUnsummarizedFiles(with("- `audioService.ts` - Audio service.", "- `audioSignal.ts` - "+TodoSummary, "- `audioWave.ts` - "+TodoSummary))
	if !reflect.DeepEqual(got, []string{"src/audio/audioSignal.ts", "src/audio/audioWave.ts"}) {
		t.Errorf("%v", got)
	}
	if got := FindUnsummarizedFiles(with("- `audioService.ts` - Audio service.")); len(got) != 0 {
		t.Errorf("%v", got)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	return root
}

func put(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(content), 0o644)
}

func gitAdd(t *testing.T, root string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", root, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestListGitFilesFiltersAndHonoursTheWorkingTree(t *testing.T) {
	root := gitRepo(t)
	put(t, root, "src/a.ts", "x")
	put(t, root, "src/gone.ts", "x")
	put(t, root, "src/notes.md", "x")
	put(t, root, "src/node_modules/x/i.js", "x")
	put(t, root, "scripts/work_dot_bat/plan-status.sh", "x")
	put(t, root, "scripts/mine.sh", "x")
	put(t, root, "e2e/t.spec.ts", "x")
	put(t, root, "other/ignored.ts", "x")
	gitAdd(t, root)
	os.Remove(filepath.Join(root, "src/gone.ts")) // tracked but deleted on disk, not yet staged
	got, err := ListGitFiles(root, []string{"src", "e2e", "scripts"}, RealGit)
	if err != nil || !reflect.DeepEqual(got, []string{"e2e/t.spec.ts", "scripts/mine.sh", "src/a.ts"}) {
		t.Errorf("%v %v", got, err)
	}
}

func TestReadStructureConfig(t *testing.T) {
	root := t.TempDir()
	if c := ReadStructureConfig(root); c.Doc != "agent_docs/02_project_structure.md" || !reflect.DeepEqual(c.Dirs, []string{"src", "e2e", "scripts", ".tools"}) {
		t.Errorf("defaults: %+v", c)
	}

	// Falls back to legacy 03_project_structure.md if 02_ does not exist on disk but 03_ does.
	put(t, root, "agent_docs/03_project_structure.md", "# Structure")
	if c := ReadStructureConfig(root); c.Doc != "agent_docs/03_project_structure.md" {
		t.Errorf("fallback to 03 failed: %+v", c)
	}
	// Once 02_ exists, it takes precedence.
	put(t, root, "agent_docs/02_project_structure.md", "# Structure")
	if c := ReadStructureConfig(root); c.Doc != "agent_docs/02_project_structure.md" {
		t.Errorf("02 precedence failed: %+v", c)
	}

	put(t, root, "work_dot_bat.config.json", `{"projectStructureDoc":"docs/s.md","projectStructureDirs":["lib","cmd"]}`)
	if c := ReadStructureConfig(root); c.Doc != "docs/s.md" || !reflect.DeepEqual(c.Dirs, []string{"lib", "cmd"}) {
		t.Errorf("configured: %+v", c)
	}
	put(t, root, "work_dot_bat.config.json", "{ nope")
	if c := ReadStructureConfig(root); c.Doc != "agent_docs/02_project_structure.md" {
		t.Errorf("an unreadable config falls back to the defaults: %+v", c)
	}

	// Test reading from projects.json when work_dot_bat.config.json is absent
	regDir := t.TempDir()
	regFile := filepath.Join(regDir, "projects.json")
	projRoot := t.TempDir()
	regContent := `{"projects":[{"name":"regproj","path":"` + filepath.ToSlash(projRoot) + `","projectStructureDoc":"agent_docs/custom.md","projectStructureDirs":["src","pkg"]}]}`
	os.WriteFile(regFile, []byte(regContent), 0o644)
	t.Setenv(projects.RegistryEnv, regFile)
	if c := ReadStructureConfig(projRoot); c.Doc != "agent_docs/custom.md" || !reflect.DeepEqual(c.Dirs, []string{"src", "pkg"}) {
		t.Errorf("registry-configured failed: %+v", c)
	}

	// Test reading from inside a linked worktree resolves the main root's config
	treeDir := filepath.Join(projRoot, ".worktrees", "feature-1")
	os.MkdirAll(treeDir, 0o755)
	os.WriteFile(filepath.Join(treeDir, ".git"), []byte("gitdir: "+filepath.ToSlash(projRoot)+"/.git/worktrees/feature-1\n"), 0o644)
	if c := ReadStructureConfig(treeDir); c.Doc != "agent_docs/custom.md" || !reflect.DeepEqual(c.Dirs, []string{"src", "pkg"}) {
		t.Errorf("worktree-configured failed: %+v", c)
	}
}

func TestStructureMainExitCodes(t *testing.T) {
	root := t.TempDir()
	docPath := filepath.Join(root, "agent_docs", "02_project_structure.md")
	put(t, root, "agent_docs/02_project_structure.md", "# Heading\n\n"+BeginMarker+"\n\n## src\n\n- `old.go` - Old summary.\n\n"+EndMarker+"\n")
	git := func(dir string, args ...string) (string, error) {
		return "src/old.go\nsrc/new.go\n", nil
	}
	put(t, root, "src/old.go", "package main")
	put(t, root, "src/new.go", "package main")

	var buf bytes.Buffer
	// Without --check: update should succeed, write the file, report unsummarized, and exit 0.
	code := StructureMain(root, nil, &buf, git)
	if code != 0 {
		t.Fatalf("expected update mode to return 0, got %d, output: %s", code, buf.String())
	}
	content, _ := os.ReadFile(docPath)
	if !strings.Contains(string(content), "new.go") {
		t.Fatalf("expected new.go in updated doc, got:\n%s", content)
	}

	// With --check: should return 1 because new.go has TODO: summarize.
	buf.Reset()
	code = StructureMain(root, []string{"--check"}, &buf, git)
	if code != 1 {
		t.Fatalf("expected --check with unsummarized to return 1, got %d", code)
	}
	if !strings.Contains(buf.String(), "TODO: summarize") {
		t.Fatalf("expected check output to mention TODO: summarize, got: %s", buf.String())
	}
}
