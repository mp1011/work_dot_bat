package managed

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRenderParseRoundTrip(t *testing.T) {
	body := "import x from './x.mjs'\nconsole.log(1)\n"
	text, err := Render("scripts/work_dot_bat/lib/a.mjs", "1.4.0", "2026-10-03", body)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(text, "\n")
	want := []string{
		"// work_dot_bat managed file — DO NOT EDIT; overwritten by `work_dot_bat sync`.",
		"// file: scripts/work_dot_bat/lib/a.mjs",
		"// version: 1.4.0",
		"// date: 2026-10-03",
		"// sha256: " + HashBody(body),
	}
	if !reflect.DeepEqual(lines[:5], want) {
		t.Errorf("header %q", lines[:5])
	}
	p := Parse(text)
	if p == nil || p.Body != body || p.Version != "1.4.0" || p.File != "scripts/work_dot_bat/lib/a.mjs" || p.Date != "2026-10-03" || p.SHA256 != HashBody(body) {
		t.Errorf("%+v", p)
	}
}

func TestShebangStaysFirstAndShellUsesHash(t *testing.T) {
	body := "#!/usr/bin/env bash\nset -e\necho hi\n"
	text, _ := Render("scripts/work_dot_bat/x.sh", "1.0.0", "2026-10-03", body)
	lines := strings.Split(text, "\n")
	if lines[0] != "#!/usr/bin/env bash" || !strings.HasPrefix(lines[1], "# work_dot_bat managed file") {
		t.Errorf("%q", lines[:3])
	}
	if p := Parse(text); p == nil || p.Body != body {
		t.Errorf("%+v", p)
	}
}

func TestMarkdownUsesHTMLComments(t *testing.T) {
	text, _ := Render("agent_docs/x.md", "1.4.0", "2026-10-03", "# T\n")
	if strings.Split(text, "\n")[2] != "<!-- version: 1.4.0 -->" {
		t.Errorf("%q", text)
	}
	if p := Parse(text); p == nil || p.Version != "1.4.0" || p.Body != "# T\n" {
		t.Errorf("%+v", p)
	}
}

func TestHashIgnoresLineEndingsAndDetectsEdits(t *testing.T) {
	if HashBody("a\r\nb\r\n") != HashBody("a\nb\n") {
		t.Error("CRLF checkouts must not read as edits")
	}
	text, _ := Render("a.mjs", "1.0.0", "2026-10-03", "one\n")
	edited := strings.Replace(text, "one", "two", 1)
	if p := Parse(edited); p == nil || HashBody(p.Body) == p.SHA256 {
		t.Error("a local edit must make the body disagree with the header hash")
	}
	if Parse("just a file\n") != nil || Parse("") != nil {
		t.Error("no header")
	}
	if _, err := Render("data.json", "1.0.0", "2026-10-03", "{}"); err == nil {
		t.Error("JSON has no comment syntax")
	}
}

func TestVersions(t *testing.T) {
	if c, _ := CompareVersions("1.2.3", "1.10.0"); c != -1 {
		t.Error("numeric compare")
	}
	if c, _ := CompareVersions("2.0.0", "1.99.99"); c != 1 {
		t.Error()
	}
	if c, _ := CompareVersions("1.2.3", "1.2.3"); c != 0 {
		t.Error()
	}
	if _, err := ParseVersion("1.2"); err == nil {
		t.Error("invalid version")
	}
	for in, want := range map[[2]string]string{{"1.2.3", "patch"}: "1.2.4", {"1.2.3", "minor"}: "1.3.0", {"1.2.3", "major"}: "2.0.0"} {
		if got, _ := BumpVersion(in[0], in[1]); got != want {
			t.Errorf("%v: %s", in, got)
		}
	}
	if _, err := BumpVersion("1.0.0", "huge"); err == nil {
		t.Error("unknown level")
	}
}

func TestDriftAndBump(t *testing.T) {
	fsys := fstest.MapFS{
		"scripts/work_dot_bat/a.sh":      {Data: []byte("a\n")},
		"scripts/work_dot_bat/b.mjs":     {Data: []byte("b\n")},
		"scripts/work_dot_bat/c.test.ts": {Data: []byte("skipped\n")},
		"manifest.json":                  {Data: []byte("{}")},
	}
	if got := ListVendorFiles(fsys); !reflect.DeepEqual(got, []string{"scripts/work_dot_bat/a.sh", "scripts/work_dot_bat/b.mjs"}) {
		t.Errorf("%v", got)
	}
	m := Manifest{Files: map[string]Entry{
		"scripts/work_dot_bat/a.sh":    {Version: "1.0.0", Date: "2026-01-01", SHA256: HashBody("old\n")}, // changed
		"scripts/work_dot_bat/gone.sh": {Version: "1.0.0", Date: "2026-01-01", SHA256: "x"},               // removed
	}}
	d, err := FindDrift(m, fsys)
	if err != nil || d.Clean() || !reflect.DeepEqual(d.Added, []string{"scripts/work_dot_bat/b.mjs"}) ||
		!reflect.DeepEqual(d.Changed, []string{"scripts/work_dot_bat/a.sh"}) || !reflect.DeepEqual(d.Removed, []string{"scripts/work_dot_bat/gone.sh"}) {
		t.Fatalf("%+v %v", d, err)
	}
	hash := func(f string) string { b, _ := ReadBody(fsys, f); return HashBody(b) }
	next, _ := ApplyBump(m, d, "minor", "2026-10-03", hash)
	if e := next.Files["scripts/work_dot_bat/a.sh"]; e.Version != "1.1.0" || e.Date != "2026-10-03" || e.SHA256 != HashBody("a\n") {
		t.Errorf("%+v", e)
	}
	if e := next.Files["scripts/work_dot_bat/b.mjs"]; e.Version != "1.0.0" {
		t.Errorf("new files start at 1.0.0: %+v", e)
	}
	if _, ok := next.Files["scripts/work_dot_bat/gone.sh"]; ok {
		t.Error("removed files are dropped")
	}
	// After recording, there is no drift.
	m2fs := fstest.MapFS{"scripts/work_dot_bat/a.sh": fsys["scripts/work_dot_bat/a.sh"], "scripts/work_dot_bat/b.mjs": fsys["scripts/work_dot_bat/b.mjs"]}
	if d2, _ := FindDrift(next, m2fs); !d2.Clean() {
		t.Errorf("%+v", d2)
	}
}
