package maintcli_test

import (
	"encoding/json"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	wdb "work_dot_bat"
	"work_dot_bat/internal/maintcli"
	"work_dot_bat/internal/managed"
	"work_dot_bat/internal/safetyhook"
)

func assets(t *testing.T) (vendor, templates fs.FS) {
	t.Helper()
	v, err := fs.Sub(wdb.Vendor, "vendor")
	if err != nil {
		t.Fatal(err)
	}
	tm, err := fs.Sub(wdb.Templates, "templates/files")
	if err != nil {
		t.Fatal(err)
	}
	return v, tm
}

type rig struct {
	t        *testing.T
	root     string
	out, err strings.Builder
	cfg      maintcli.Config
}

func newRig(t *testing.T) *rig {
	v, tm := assets(t)
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)
	r := &rig{t: t, root: root}
	r.cfg = maintcli.Config{
		Root: root, ProjectName: "Acme", Out: &r.out, Err: &r.err, Vendor: v, Templates: tm,
		Now: func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) },
	}
	return r
}

func (r *rig) read(rel string) string {
	b, _ := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(rel)))
	return string(b)
}

func (r *rig) has(rel string) bool {
	_, err := os.Stat(filepath.Join(r.root, filepath.FromSlash(rel)))
	return err == nil
}

func (r *rig) write(rel, content string) {
	full := filepath.Join(r.root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(full), 0o755)
	os.WriteFile(full, []byte(content), 0o644)
}

func (r *rig) reset() { r.out.Reset(); r.err.Reset() }

// What gets copied into target repos is a contract: recorded, versioned, header-able and living
// under the managed folder.
func TestEmbeddedVendorMatchesItsManifest(t *testing.T) {
	v, _ := assets(t)
	m, err := managed.ReadManifest(v)
	if err != nil || len(m.Files) == 0 {
		t.Fatalf("manifest: %v (%d files)", err, len(m.Files))
	}
	d, err := managed.FindDrift(m, v)
	if err != nil || !d.Clean() {
		t.Fatalf("vendored files changed without a recorded bump (run `work_dot_bat vendor-bump`): %+v %v", d, err)
	}
	dateRE := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	shaRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for file, e := range m.Files {
		if c, err := managed.CompareVersions(e.Version, "0.0.0"); err != nil || c < 0 {
			t.Errorf("%s: version %q", file, e.Version)
		}
		if !dateRE.MatchString(e.Date) || !shaRE.MatchString(e.SHA256) {
			t.Errorf("%s: %+v", file, e)
		}
	}
	for _, file := range managed.ListVendorFiles(v) {
		if !strings.HasPrefix(file, "scripts/work_dot_bat/") {
			t.Errorf("%s must live under scripts/work_dot_bat/", file)
		}
		if managed.CommentStyle(file) == nil {
			t.Errorf("%s has no comment syntax for a managed header", file)
		}
		body, _ := managed.ReadBody(v, file)
		if managed.Parse(body) != nil {
			t.Errorf("%s must be stored without a header", file)
		}
		if strings.HasSuffix(file, ".sh") && strings.Contains(body, "\r") {
			t.Errorf("%s must use LF line endings", file)
		}
	}
	for _, name := range []string{"plan-status", "plan-claim", "list-tasks", "worktree-info", "run-checks"} {
		if _, ok := m.Files["scripts/work_dot_bat/"+name+".sh"]; !ok {
			t.Errorf("missing wrapper for %s", name)
		}
	}
}

func TestVendorCheckAndBumpOnDisk(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	os.MkdirAll(filepath.Join(vendor, "scripts/work_dot_bat"), 0o755)
	os.WriteFile(filepath.Join(vendor, "scripts/work_dot_bat/a.sh"), []byte("echo a\n"), 0o644)
	var out strings.Builder
	if code := maintcli.VendorCheck(dir, &out); code != 1 || !strings.Contains(out.String(), "new      scripts/work_dot_bat/a.sh") {
		t.Fatalf("%d %s", code, out.String())
	}
	out.Reset()
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	if code := maintcli.VendorBump(dir, nil, now, &out); code != 0 || !strings.Contains(out.String(), "Recorded (patch bump, 2026-10-03):") {
		t.Fatalf("%d %s", code, out.String())
	}
	out.Reset()
	if code := maintcli.VendorCheck(dir, &out); code != 0 || !strings.Contains(out.String(), "matches the vendored files") {
		t.Fatalf("%d %s", code, out.String())
	}
	os.WriteFile(filepath.Join(vendor, "scripts/work_dot_bat/a.sh"), []byte("echo b\n"), 0o644)
	out.Reset()
	maintcli.VendorBump(dir, []string{"--minor"}, now, &out)
	b, _ := os.ReadFile(filepath.Join(vendor, "manifest.json"))
	var m managed.Manifest
	json.Unmarshal(b, &m)
	if m.Files["scripts/work_dot_bat/a.sh"].Version != "1.1.0" || !strings.HasSuffix(string(b), "}\n") {
		t.Errorf("%s", b)
	}
	out.Reset()
	if maintcli.VendorBump(dir, nil, now, &out); out.String() != "Nothing to bump.\n" {
		t.Errorf("%q", out.String())
	}
}

func TestSyncDryRunThenYes(t *testing.T) {
	r := newRig(t)
	if code := maintcli.Sync(r.cfg, []string{"--dry-run"}); code != 1 {
		t.Errorf("a dry run with stale files exits non-zero, got %d", code)
	}
	if !strings.Contains(r.out.String(), "Would write: scripts/work_dot_bat/plan-status.sh (new)") || r.has("scripts") {
		t.Errorf("%s", r.out.String())
	}
	r.reset()
	if code := maintcli.Sync(r.cfg, []string{"--yes"}); code != 0 {
		t.Fatalf("exit %d: %s%s", code, r.out.String(), r.err.String())
	}
	v, _ := assets(t)
	m, _ := managed.ReadManifest(v)
	for file, e := range m.Files {
		p := managed.Parse(r.read(file))
		if p == nil || p.SHA256 != e.SHA256 || p.Version != e.Version {
			t.Errorf("%s: %+v", file, p)
		}
	}
	if !strings.HasSuffix(r.out.String(), "Changes are in the working tree only; review and commit them yourself.\n") {
		t.Errorf("%s", r.out.String())
	}
	r.reset()
	if code := maintcli.Sync(r.cfg, nil); code != 0 || r.out.String() != "Everything is up to date.\n" {
		t.Errorf("%d %q", code, r.out.String())
	}
}

func TestSyncDoesNotClobberLocalEditsWithoutYes(t *testing.T) {
	r := newRig(t)
	maintcli.Sync(r.cfg, []string{"--yes"})
	target := "scripts/work_dot_bat/plan-status.sh"
	edited := strings.Replace(r.read(target), "exec ", "exec echo LOCAL ", 1)
	if edited == r.read(target) {
		edited = r.read(target) + "# local edit\n"
	}
	r.write(target, edited)
	r.reset()
	if code := maintcli.Sync(r.cfg, nil); code != 1 {
		t.Errorf("a skipped file exits non-zero, got %d", code)
	}
	if r.read(target) != edited || !strings.Contains(r.out.String(), "Skipped: "+target+" (edited locally)") {
		t.Errorf("%s", r.out.String())
	}
	r.reset()
	if code := maintcli.Sync(r.cfg, []string{"--yes"}); code != 0 || r.read(target) == edited {
		t.Errorf("--yes should overwrite: exit %d", code)
	}
}

func TestSyncRefusesAWorktree(t *testing.T) {
	r := newRig(t)
	os.Remove(filepath.Join(r.root, ".git"))
	r.write(".git", "gitdir: /elsewhere\n")
	if code := maintcli.Sync(r.cfg, nil); code != 1 || !strings.Contains(r.err.String(), "is not a main checkout") {
		t.Errorf("%d %s", code, r.err.String())
	}
}

func TestBootstrapDryRunWritesNothingThenCreatesEverythingOnce(t *testing.T) {
	r := newRig(t)
	if code := maintcli.Bootstrap(r.cfg, []string{"--dry-run"}); code != 0 {
		t.Fatalf("%d %s", code, r.err.String())
	}
	if !strings.Contains(r.out.String(), "Would create: work_dot_bat.config.json") || r.has("agent_docs") || r.has("work_dot_bat.config.json") || r.has(".claude") {
		t.Errorf("a dry run must write nothing:\n%s", r.out.String())
	}
	if strings.Contains(r.out.String(), "Nothing is committed") {
		t.Error("a dry run does not claim to have written")
	}

	r.reset()
	if code := maintcli.Bootstrap(r.cfg, nil); code != 0 {
		t.Fatalf("%d %s", code, r.err.String())
	}
	for _, want := range []string{
		"work_dot_bat.config.json", "agent_docs/00_start_here.md", "agent_docs/01_overview.md", "agent_docs/02_project_structure.md",
		"agent_docs/plan_template.md", "agent_docs/instructions/high_trust/do-work-yolo.md", "agent_docs/plans/.gitkeep",
		"agent_docs/monolith_baseline.json", "agent_docs/duplication_baseline.json", "agent_docs/structural_duplication_baseline.json",
		".claude/settings.json", ".git/hooks/pre-push", "scripts/work_dot_bat/plan-status.sh", "scripts/work_dot_bat/typescript/check-source-assertions.mjs",
		"CLAUDE.md", "GEMINI.md",
	} {
		if !r.has(want) {
			t.Errorf("missing %s", want)
		}
	}
	// Placeholders are all filled from the new config; the project name and npm commands land in the docs.
	if left := regexp.MustCompile(`\{\{[a-z_]+\}\}`).FindString(r.read("agent_docs/00_start_here.md")); left != "" {
		t.Errorf("unfilled placeholder %s", left)
	}
	if doc := r.read("agent_docs/00_start_here.md"); !strings.Contains(doc, "AGENT DOCS") || !strings.Contains(doc, "Acme") || !strings.Contains(doc, "`npm install`") {
		t.Errorf("%s", doc)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(r.read("work_dot_bat.config.json")), &cfg); err != nil || cfg["projectStructureDoc"] != "agent_docs/02_project_structure.md" {
		t.Errorf("config %v %v", cfg, err)
	}
	if !strings.Contains(r.read(".gitignore"), ".work_dot_bat/") {
		t.Errorf(".gitignore:\n%s", r.read(".gitignore"))
	}
	if hook := r.read(".git/hooks/pre-push"); !strings.HasPrefix(hook, "#!/bin/sh\n") || !strings.Contains(hook, "scripts/work_dot_bat/run-checks.sh") {
		t.Errorf("hook %q", hook)
	}
	if got := r.read("agent_docs/monolith_baseline.json"); got != "{\n  \"maxLines\": 500,\n  \"entries\": {}\n}\n" {
		t.Errorf("%q", got)
	}
	var settings struct {
		Permissions struct{ Deny []string }
		Hooks       struct {
			PreToolUse []struct{ Hooks []struct{ Command string } }
		}
	}
	json.Unmarshal([]byte(r.read(".claude/settings.json")), &settings)
	if len(settings.Permissions.Deny) != len(safetyhook.DenyRules) || settings.Hooks.PreToolUse[0].Hooks[0].Command != safetyhook.HookCommand {
		t.Errorf("settings %+v", settings)
	}
	if !strings.Contains(r.out.String(), "Nothing is committed. Review the new files and commit them yourself.") {
		t.Errorf("%s", r.out.String())
	}

	// A second run is a no-op.
	r.reset()
	if code := maintcli.Bootstrap(r.cfg, nil); code != 0 {
		t.Fatal(code)
	}
	if strings.Contains(r.out.String(), "Created:") || strings.Contains(r.out.String(), "Updated:") || strings.Contains(r.out.String(), "Installed:") {
		t.Errorf("second run changed things:\n%s", r.out.String())
	}
}

func TestBootstrapNeverOverwritesAndMergesSettings(t *testing.T) {
	r := newRig(t)
	r.write("agent_docs/00_start_here.md", "MY OWN START HERE\n")
	r.write("CLAUDE.md", "MINE\n")
	r.write(".claude/settings.json", `{"model":"opus","permissions":{"deny":["Bash(rm*)"]}}`)
	r.write(".gitignore", "node_modules/")
	r.write("work_dot_bat.config.json", `{"lightTest":{"cmd":"go","args":["test","./..."]},"projectStructureDoc":"docs/structure.md","checks":["monolith"]}`)
	r.write("docs/structure.md", "# my structure\n")
	if code := maintcli.Bootstrap(r.cfg, nil); code != 0 {
		t.Fatalf("%d %s", code, r.err.String())
	}
	if r.read("agent_docs/00_start_here.md") != "MY OWN START HERE\n" || r.read("CLAUDE.md") != "MINE\n" || r.read("docs/structure.md") != "# my structure\n" {
		t.Error("existing files must never be overwritten")
	}
	if !strings.Contains(r.out.String(), "Skipped (already exists): agent_docs/00_start_here.md") || !strings.Contains(r.out.String(), "Skipped (already exists): docs/structure.md") {
		t.Errorf("%s", r.out.String())
	}
	if r.has("agent_docs/02_project_structure.md") {
		t.Error("a configured structure doc location means no duplicate at the default path")
	}
	if r.has("agent_docs/duplication_baseline.json") || !r.has("agent_docs/monolith_baseline.json") {
		t.Error("only the enabled checks get baselines")
	}
	var s map[string]any
	json.Unmarshal([]byte(r.read(".claude/settings.json")), &s)
	deny := s["permissions"].(map[string]any)["deny"].([]any)
	if s["model"] != "opus" || deny[0] != "Bash(rm*)" || len(deny) != 1+len(safetyhook.DenyRules) {
		t.Errorf("settings %v", s)
	}
	if got := r.read(".gitignore"); got != "node_modules/\n\n# work_dot_bat runtime state (claims, session ledger, logs)\n.work_dot_bat/\n" {
		t.Errorf("%q", got)
	}
	if strings.Contains(r.read("work_dot_bat.config.json"), "npm") {
		t.Error("an existing config must stay untouched")
	}
	// The typescript prerequisite is not needed when only monolith is enabled.
	if strings.Contains(r.out.String(), "import `typescript`") {
		t.Errorf("%s", r.out.String())
	}
}

func TestBootstrapEdgeCases(t *testing.T) {
	r := newRig(t)
	r.write(".claude/settings.json", "{ not json")
	maintcli.Bootstrap(r.cfg, nil)
	if r.read(".claude/settings.json") != "{ not json" || !strings.Contains(r.out.String(), "Manual step: .claude/settings.json — exists but is not valid JSON; merge the safety hook by hand") {
		t.Errorf("%s", r.out.String())
	}

	h := newRig(t)
	h.write(".husky/_/h", "")
	maintcli.Bootstrap(h.cfg, nil)
	if h.has(".git/hooks/pre-push") || !strings.Contains(h.out.String(), "husky manages hooks here; make its pre-push hook run scripts/work_dot_bat/run-checks.sh") {
		t.Errorf("%s", h.out.String())
	}

	p := newRig(t)
	p.write(".git/hooks/pre-push", "#!/bin/sh\nmy hook\n")
	maintcli.Bootstrap(p.cfg, nil)
	if p.read(".git/hooks/pre-push") != "#!/bin/sh\nmy hook\n" {
		t.Error("an existing pre-push hook must not be replaced")
	}

	bad := newRig(t)
	bad.write("work_dot_bat.config.json", `{"checks":["nope"]}`)
	if code := maintcli.Bootstrap(bad.cfg, nil); code != 1 || !strings.Contains(bad.err.String(), "Invalid work_dot_bat.config.json") || bad.has("agent_docs") {
		t.Errorf("%d %s", code, bad.err.String())
	}

	wt := newRig(t)
	os.Remove(filepath.Join(wt.root, ".git"))
	wt.write(".git", "gitdir: /x\n")
	if code := maintcli.Bootstrap(wt.cfg, nil); code != 1 || !strings.Contains(wt.err.String(), "is not a main git checkout") {
		t.Errorf("%d %s", code, wt.err.String())
	}

	ts := newRig(t)
	maintcli.Bootstrap(ts.cfg, nil)
	if !strings.Contains(ts.out.String(), "Note: The structural-duplication and source-assertions checks import `typescript`") {
		t.Errorf("a repo without typescript should be told:\n%s", ts.out.String())
	}
	ok := newRig(t)
	ok.write("package.json", `{"devDependencies":{"typescript":"^5"}}`)
	maintcli.Bootstrap(ok.cfg, nil)
	if strings.Contains(ok.out.String(), "import `typescript`") {
		t.Errorf("%s", ok.out.String())
	}
}

func TestMigrateStateAndRewritePathsCommands(t *testing.T) {
	r := newRig(t)
	r.write(".claims/x.json", "{}")
	if code := maintcli.MigrateState(r.cfg, nil); code != 0 || !strings.Contains(r.out.String(), "Would move: .claims -> .work_dot_bat/claims") || !r.has(".claims/x.json") {
		t.Errorf("%d %s", code, r.out.String())
	}
	r.reset()
	maintcli.MigrateState(r.cfg, []string{"--apply"})
	if !r.has(".work_dot_bat/claims/x.json") || r.has(".claims") || !strings.Contains(r.out.String(), "Moved: .claims -> .work_dot_bat/claims") {
		t.Errorf("%s", r.out.String())
	}

	// rewrite_paths needs a git repo's tracked files; use a real one.
	g := newRig(t)
	os.Remove(filepath.Join(g.root, ".git"))
	if err := exec("git", g.root, "init", "-q"); err != nil {
		t.Skip("git not available")
	}
	g.write("README.md", "run scripts/plan-status.sh now\n")
	exec("git", g.root, "add", ".")
	if code := maintcli.RewritePaths(g.cfg, nil); code != 0 || !strings.Contains(g.out.String(), "Would rewrite: README.md (1)") ||
		!strings.Contains(g.out.String(), "1 file(s), 1 reference(s). Dry run: re-run with --apply.") {
		t.Errorf("%d %s", code, g.out.String())
	}
	g.reset()
	maintcli.RewritePaths(g.cfg, []string{"--apply"})
	if g.read("README.md") != "run scripts/work_dot_bat/plan-status.sh now\n" || !strings.Contains(g.out.String(), "1 file(s), 1 reference(s) rewritten.") {
		t.Errorf("%q %s", g.read("README.md"), g.out.String())
	}
}

func TestSafetyHookCommand(t *testing.T) {
	var errOut strings.Builder
	call := func(cmd string) int {
		errOut.Reset()
		b, _ := json.Marshal(map[string]any{"tool_input": map[string]any{"command": cmd}})
		return maintcli.SafetyHook(strings.NewReader(string(b)), &errOut)
	}
	if code := call("git status"); code != 0 || errOut.String() != "" {
		t.Errorf("%d %q", code, errOut.String())
	}
	if code := call("work_dot_bat work project=x"); code != 2 || strings.TrimSpace(errOut.String()) != safetyhook.WorkMessage {
		t.Errorf("%d %q", code, errOut.String())
	}
	if code := call("node src/prepare_for_push.js"); code != 2 || strings.TrimSpace(errOut.String()) != safetyhook.PushMessage {
		t.Errorf("%d %q", code, errOut.String())
	}
	if code := maintcli.SafetyHook(strings.NewReader("garbage"), &errOut); code != 0 {
		t.Errorf("garbage input must be allowed, got %d", code)
	}
}

func exec(name, dir string, args ...string) error {
	cmd := osexec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}
