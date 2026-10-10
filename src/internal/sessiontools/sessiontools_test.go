package sessiontools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"work_dot_bat/src/internal/agentsession"
)

type rig struct {
	root string
	out  strings.Builder
	err  strings.Builder
}

func (r *rig) cfg() Config {
	return Config{Root: r.root, Out: &r.out, Err: &r.err, Now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }}
}

func newRig(t *testing.T, ledger string) *rig {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".work_dot_bat", "logs"), 0o755)
	if ledger != "" {
		os.WriteFile(filepath.Join(root, ".work_dot_bat", "session_ledger.json"), []byte(ledger), 0o644)
	}
	return &rig{root: root}
}

const log = `starting
view_file AbsolutePath: "C:/r/src/a.ts"
Reading file "C:/r/src/b.ts"
Reading file C:/r/src/a.ts
this line is not json {{{ and not a match
replace_file_content TargetFile: 'C:/r/src/a.ts'
write_to_file TargetFile C:/r/new.txt
Running command: npm test
run_command CommandLine: "git status"
BLOCKED by something
Blocked — Needs User Input
`

func TestParseLogContent(t *testing.T) {
	a := agentsession.ParseLogContent(log)
	eq := func(name string, got, want []string) {
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: %v want %v", name, got, want)
		}
	}
	eq("read", a.FilesRead, []string{"C:/r/src/a.ts", "C:/r/src/b.ts"}) // each once, first-seen order
	eq("edited", a.FilesEdited, []string{"C:/r/src/a.ts", "C:/r/new.txt"})
	eq("commands", a.CommandsRun, []string{"npm test", "git status"})
	if !a.StepZeroBlocked || !a.Escalated {
		t.Errorf("%+v", a)
	}
	if b := agentsession.ParseLogContent(""); b.StepZeroBlocked || b.Escalated || len(b.FilesRead) != 0 {
		t.Errorf("%+v", b)
	}
	if c := agentsession.ParseLogContent("a claim held by another process here"); !c.StepZeroBlocked {
		t.Error("claim-held text should flag step zero")
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[[2]string]string{
		{"2026-10-03T12:00:00.000Z", "2026-10-03T12:02:05.000Z"}: "2m 5s",
		{"2026-10-03T12:00:00.000Z", "2026-10-03T12:00:09.000Z"}: "9s",
		{"2026-10-03T12:00:00.000Z", "2026-10-03T11:00:00.000Z"}: "N/A",
		{"", "2026-10-03T11:00:00.000Z"}:                         "N/A",
		{"junk", "2026-10-03T11:00:00.000Z"}:                     "N/A",
	}
	for in, want := range cases {
		if got := agentsession.FormatDuration(in[0], in[1]); got != want {
			t.Errorf("%v: %q want %q", in, got, want)
		}
	}
}

func TestAnalyzeLatestGolden(t *testing.T) {
	r := newRig(t, `[
  {"sessionName":"older","plan":"plans/o.md","status":"completed"},
  {"sessionName":"2026-10-03_12-00-00__x","plan":"agent_docs/plans/x.md","agent":"claude","status":"failed","exitCode":1,
   "startedAt":"2026-10-03T12:00:00.000Z","endedAt":"2026-10-03T12:02:05.000Z","sessionId":"sid","logFile":".work_dot_bat/logs/x.log"}
]`)
	os.WriteFile(filepath.Join(r.root, ".work_dot_bat", "logs", "x.log"), []byte(log), 0o644)
	if code := Analyze(r.cfg(), nil); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Analyzing latest session (2026-10-03_12-00-00__x):\n\n" +
		"===================================================\n" +
		"Session:    2026-10-03_12-00-00__x\n" +
		"Plan:       agent_docs/plans/x.md\n" +
		"Agent:      claude\n" +
		"Status:     failed (Exit Code: 1)\n" +
		"Duration:   2m 5s (2026-10-03T12:00:00.000Z -> 2026-10-03T12:02:05.000Z)\n" +
		"Session ID: sid\n" +
		"Log File:   .work_dot_bat/logs/x.log\n" +
		"---------------------------------------------------\n" +
		"Files Read (2):\n  - C:/r/src/a.ts\n  - C:/r/src/b.ts\n" +
		"Files Edited (2):\n  - C:/r/src/a.ts\n  - C:/r/new.txt\n" +
		"Commands Run (2):\n  - npm test\n  - git status\n" +
		"---------------------------------------------------\n" +
		"Step Zero Blocked:      YES\n" +
		"Testability Escalation: YES\n" +
		"===================================================\n\n"
	if r.out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", r.out.String(), want)
	}
}

func TestAnalyzeMissingFieldsAndSelectors(t *testing.T) {
	r := newRig(t, `[{"sessionName":"s1","plan":"agent_docs/plans/a/b.md"},{"sessionName":"s2","plan":"plans/a/b.md"},{"sessionName":"s3"}]`)
	Analyze(r.cfg(), []string{"s3"})
	for _, want := range []string{`Found session matching name "s3":`, "Plan:       N/A", "Agent:      N/A", "Status:     unknown\n",
		"Duration:   N/A (N/A -> N/A)", "(none detected)", "Step Zero Blocked:      no"} {
		if !strings.Contains(r.out.String(), want) {
			t.Errorf("missing %q:\n%s", want, r.out.String())
		}
	}
	r2 := newRig(t, `[{"sessionName":"s1","plan":"agent_docs/plans/a/b.md"},{"sessionName":"s2","plan":"plans/a/b.md"},{"sessionName":"s3"}]`)
	Analyze(r2.cfg(), []string{"plans/a/b.md"})
	if !strings.Contains(r2.out.String(), `Found 2 session(s) for plan "plans/a/b.md":`) || strings.Count(r2.out.String(), "Session:    ") != 2 {
		t.Errorf("%s", r2.out.String())
	}
	r3 := newRig(t, `[{"sessionName":"s1"}]`)
	Analyze(r3.cfg(), []string{"nothing-like-this"})
	if r3.out.String() != "No sessions found matching \"nothing-like-this\".\n" {
		t.Errorf("%q", r3.out.String())
	}
	r4 := newRig(t, "")
	Analyze(r4.cfg(), nil)
	if r4.out.String() != "No sessions recorded yet in .work_dot_bat/session_ledger.json.\n" {
		t.Errorf("%q", r4.out.String())
	}
	r5 := newRig(t, "[]")
	Analyze(r5.cfg(), []string{"-h"})
	if !strings.HasPrefix(r5.out.String(), "\nUsage:\n  work_dot_bat analyze_session project=<name> [--latest]") {
		t.Errorf("%q", r5.out.String())
	}
}

func TestAnalyzeToleratesMalformedLedgerAndLog(t *testing.T) {
	r := newRig(t, "{ this is not json")
	if code := Analyze(r.cfg(), nil); code != 0 || !strings.Contains(r.out.String(), "No sessions recorded yet") {
		t.Errorf("%d %q", code, r.out.String())
	}
}

const bounce = `{"type":"assistant","message":{"content":[{"type":"text","text":"You've hit your session limit · resets 1:30pm (America/New_York)"}]}}` + "\n"

func TestReclassifyDryRunThenWriteChangesOnlyBounces(t *testing.T) {
	ledger := "[\n" +
		"  {\n    \"sessionName\": \"bounced\",\n    \"agent\": \"claude\",\n    \"sessionId\": \"sid-b\",\n    \"status\": \"completed\",\n    \"exitCode\": 0,\n    \"startedAt\": \"2026-10-03T10:00:00.000Z\",\n    \"endedAt\": \"2026-10-03T10:00:30.000Z\"\n  },\n" +
		"  {\n    \"sessionName\": \"fine\",\n    \"agent\": \"claude\",\n    \"sessionId\": \"sid-f\",\n    \"status\": \"completed\",\n    \"exitCode\": 0\n  },\n" +
		"  {\n    \"sessionName\": \"failed\",\n    \"agent\": \"claude\",\n    \"sessionId\": \"sid-b\",\n    \"status\": \"failed\",\n    \"exitCode\": 1\n  }\n]"
	r := newRig(t, ledger)
	projects := t.TempDir()
	os.MkdirAll(filepath.Join(projects, "C--wt"), 0o755)
	os.WriteFile(filepath.Join(projects, "C--wt", "sid-b.jsonl"), []byte(bounce), 0o644)
	os.WriteFile(filepath.Join(projects, "C--wt", "sid-f.jsonl"), []byte(`{"type":"assistant","message":{"content":"all done"}}`+"\n"), 0o644)
	file := filepath.Join(r.root, ".work_dot_bat", "session_ledger.json")

	if code := Reclassify(r.cfg(), []string{"--projects-dir", projects}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got, _ := os.ReadFile(file); string(got) != ledger {
		t.Fatal("a dry run must not touch the ledger")
	}
	want := "3 session(s) in " + file + "; 1 bounced off a usage limit:\n  bounced\n\nDry run — re-run with --write to relabel them `rate-limited`.\n"
	if r.out.String() != want {
		t.Errorf("got %q\nwant %q", r.out.String(), want)
	}

	r2 := &rig{root: r.root}
	Reclassify(r2.cfg(), []string{"--projects-dir", projects, "--write"})
	got, _ := os.ReadFile(file)
	// Only the bounced row changed: status flipped in place and a resetAt appended; every other
	// byte of the file, including key order and the untouched rows, is identical.
	wantFile := strings.Replace(ledger, "\"sessionName\": \"bounced\",\n    \"agent\": \"claude\",\n    \"sessionId\": \"sid-b\",\n    \"status\": \"completed\",\n    \"exitCode\": 0,\n    \"startedAt\": \"2026-10-03T10:00:00.000Z\",\n    \"endedAt\": \"2026-10-03T10:00:30.000Z\"",
		"\"sessionName\": \"bounced\",\n    \"agent\": \"claude\",\n    \"sessionId\": \"sid-b\",\n    \"status\": \"rate-limited\",\n    \"exitCode\": 0,\n    \"startedAt\": \"2026-10-03T10:00:00.000Z\",\n    \"endedAt\": \"2026-10-03T10:00:30.000Z\",\n    \"resetAt\": \"2026-10-03T17:30:00.000Z\"", 1)
	if string(got) != wantFile {
		t.Errorf("rewritten ledger:\n%s\nwant:\n%s", got, wantFile)
	}
	if !strings.HasSuffix(r2.out.String(), "\nRewrote "+file+".\n") {
		t.Errorf("%q", r2.out.String())
	}
}

func TestReclassifyOrphans(t *testing.T) {
	ledger := `[
  {"sessionName":"old","plan":"plans/a.md","status":"in-progress","startedAt":"2026-10-01T10:00:00.000Z"},
  {"sessionName":"old-claimed","plan":"plans/live.md","status":"in-progress","startedAt":"2026-10-01T10:00:00.000Z"},
  {"sessionName":"recent","plan":"plans/b.md","status":"in-progress","startedAt":"2026-10-03T11:00:00.000Z"},
  {"sessionName":"done","status":"completed"},
  {"sessionName":"nodate","plan":"plans/c.md","status":"in-progress"}
]`
	r := newRig(t, ledger)
	// A live claim on plans/live.md keeps its row.
	os.MkdirAll(filepath.Join(r.root, ".work_dot_bat", "claims"), 0o755)
	host, _ := os.Hostname()
	claim := `{"scope":"plan","id":"plans/live.md","owner":{"kind":"launcher","label":"w","pid":` + itoa(os.Getpid()) + `,"host":"` + host + `"},"branch":"live","worktree":null,"acquiredAt":"` +
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z") + `","ttlMs":14400000}`
	os.WriteFile(filepath.Join(r.root, ".work_dot_bat", "claims", "plan--plans-live.md.json"), []byte(claim), 0o644)

	Reclassify(r.cfg(), []string{"--orphans", "--write"})
	rows := agentsession.ReadLedger(filepath.Join(r.root, ".work_dot_bat", "session_ledger.json"))
	status := map[string]any{}
	for _, e := range rows {
		status[e["sessionName"].(string)] = e["status"]
	}
	want := map[string]any{"old": "abandoned", "old-claimed": "in-progress", "recent": "in-progress", "done": "completed", "nodate": "abandoned"}
	for k, v := range want {
		if status[k] != v {
			t.Errorf("%s: %v want %v (%s)", k, status[k], v, r.out.String())
		}
	}
	if !strings.Contains(r.out.String(), "5 session(s) in") || !strings.Contains(r.out.String(), "2 orphaned in-progress session(s):") {
		t.Errorf("%s", r.out.String())
	}
	for _, e := range rows {
		if e["sessionName"] == "old" && e["endedAt"] != "2026-10-03T12:00:00.000Z" {
			t.Errorf("endedAt %v", e["endedAt"])
		}
	}
}

func TestReclassifyErrorsAndHelp(t *testing.T) {
	r := newRig(t, "")
	if code := Reclassify(r.cfg(), nil); code != 1 || !strings.Contains(r.err.String(), "No session ledger at ") {
		t.Errorf("%d %q", code, r.err.String())
	}
	r2 := newRig(t, "[]")
	if code := Reclassify(r2.cfg(), []string{"--help"}); code != 0 || !strings.HasPrefix(r2.out.String(), "Usage: work_dot_bat reclassify_sessions") {
		t.Errorf("%d %q", code, r2.out.String())
	}
	r3 := newRig(t, "[]")
	Reclassify(r3.cfg(), nil)
	if !strings.Contains(r3.out.String(), "0 session(s) in") || strings.Contains(r3.out.String(), "Dry run") {
		t.Errorf("%q", r3.out.String())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
