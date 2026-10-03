package agentsession

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionNameAndID(t *testing.T) {
	ts := time.Date(2026, 10, 3, 9, 5, 7, 0, time.Local)
	if got := FormatSessionName("agent_docs/plans/a/b c.md", ts, nil); got != "2026-10-03_09-05-07__a__b_c" {
		t.Error(got)
	}
	if got := FormatSessionName("plans/x.md", ts, Attempt(2)); got != "2026-10-03_09-05-07__x_attempt-2" {
		t.Error(got)
	}
	// Reference value computed by the Node implementation.
	if got := SessionID("2026-10-03_12-00-00__initial_port__04c"); got != "4ba6ed8f-fa2c-5a01-9e5a-a5161fab7ce2" {
		t.Error(got)
	}
}

func TestLedgerStartEndPreservesUnknownFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state", "ledger.json")
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte(`[{"sessionName":"old","extra":"keep"}]`), 0o644)
	if _, err := RecordStart(file, Entry{"sessionName": "s1", "plan": "plans/x.md", "agent": "claude"}, 42, now); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordEnd(file, Entry{"sessionName": "s1", "exitCode": 1, "status": "failed"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ledger := ReadLedger(file)
	if len(ledger) != 2 || ledger[0]["extra"] != "keep" {
		t.Fatalf("%v", ledger)
	}
	r := ledger[1]
	if r["pid"] != float64(42) || r["startedAt"] != "2026-10-03T12:00:00.000Z" || r["endedAt"] != "2026-10-03T12:01:00.000Z" ||
		r["status"] != "failed" || r["plan"] != "plans/x.md" {
		t.Fatalf("%v", r)
	}
	if got := FindSessionsForPlan(file, "agent_docs/plans/x.md"); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	// RecordEnd for an unknown session appends a row.
	RecordEnd(file, Entry{"sessionName": "ghost", "status": "failed"}, now)
	if len(ReadLedger(file)) != 3 {
		t.Fatal("ghost row not appended")
	}
}

func TestParseRateLimitReset(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	got, ok := ParseRateLimitReset("You've hit your session limit · resets 1:30pm (America/New_York)", now)
	if !ok || !got.Equal(time.Date(2026, 10, 3, 17, 30, 0, 0, time.UTC)) {
		t.Errorf("%v %v", got, ok)
	}
	// 3am Tokyo today (18:00Z previous day) has passed, so the next one is tomorrow's.
	got, ok = ParseRateLimitReset("hit your usage limit resets 3am (Asia/Tokyo)", now)
	if !ok || !got.Equal(time.Date(2026, 10, 3, 18, 0, 0, 0, time.UTC)) {
		t.Errorf("%v %v", got, ok)
	}
	if _, ok := ParseRateLimitReset("resets 3am (Mars/Olympus)", now); ok {
		t.Error("unknown zone should not parse")
	}
	if _, ok := ParseRateLimitReset("no time here", now); ok {
		t.Error("no match")
	}
	if got, _ := ParseRateLimitReset("resets 12am (UTC)", now); !got.Equal(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("12am: %v", got)
	}
}

func TestClassifyExit(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	msg := "You've hit your session limit · resets 1:30pm (America/New_York)"
	if v := ClassifyExit(0, "", msg, now); v.Status != RateLimited || v.ResetAt == nil {
		t.Errorf("%+v", v)
	}
	if v := ClassifyExit(0, msg, "", now); v.Status != RateLimited {
		t.Errorf("%+v", v)
	}
	long := msg + " " + string(make([]byte, 0)) + stringsRepeat("x", 400)
	if v := ClassifyExit(0, "", long, now); v.Status != Completed {
		t.Errorf("a long message that quotes the limit is not a bounce: %+v", v)
	}
	if v := ClassifyExit(1, "", "", now); v.Status != Failed {
		t.Errorf("%+v", v)
	}
	if v := ClassifyExit(0, "", "", now); v.Status != Completed || v.ResetAt != nil {
		t.Errorf("%+v", v)
	}
}

func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func TestRateLimitPause(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if RateLimitPause(nil, now) != 30*time.Minute {
		t.Error("fallback")
	}
	r := now.Add(10 * time.Minute)
	if RateLimitPause(&r, now) != 11*time.Minute {
		t.Error("reset + buffer")
	}
	past := now.Add(-time.Hour)
	if RateLimitPause(&past, now) != time.Minute {
		t.Error("past reset")
	}
}

func TestClassifySessionFromClaudeTranscript(t *testing.T) {
	projects := t.TempDir()
	os.MkdirAll(filepath.Join(projects, "C--some-worktree"), 0o755)
	transcript := `{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"You've hit your session limit · resets 1:30pm (America/New_York)"}]}}
`
	os.WriteFile(filepath.Join(projects, "C--some-worktree", "sid-1.jsonl"), []byte(transcript), 0o644)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	v := ClassifySession("claude", "sid-1", "", 0, Dirs{Projects: projects}, now)
	if v.Status != RateLimited || v.ResetAt == nil {
		t.Errorf("%+v", v)
	}
	if v := ClassifySession("claude", "other", "", 0, Dirs{Projects: projects}, now); v.Status != Completed {
		t.Errorf("no transcript => by exit code: %+v", v)
	}
}

func TestClassifySessionFromAgyLog(t *testing.T) {
	brain := t.TempDir()
	conv := "0a1b2c3d-0000-4000-8000-123456789abc"
	dir := filepath.Join(brain, conv, ".system_generated", "logs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "transcript.jsonl"),
		[]byte(`{"source":"MODEL","content":"You've hit your usage limit resets 3pm (UTC)"}`+"\n"), 0o644)
	logFile := filepath.Join(t.TempDir(), "agy.log")
	os.WriteFile(logFile, []byte("x\nCreated conversation "+conv+"\n"), 0o644)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	v := ClassifySession("a", "", logFile, 0, Dirs{AgyBrain: brain}, now)
	if v.Status != RateLimited || v.ResetAt == nil || !v.ResetAt.Equal(time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("%+v", v)
	}
	if got := FindLatestAgyTranscriptPath(brain); got == "" {
		t.Error("latest transcript not found")
	}
}

func TestRewritingTheLedgerKeepsKeyOrderAndFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "ledger.json")
	original := "[\n  {\n    \"zeta\": 1,\n    \"sessionName\": \"old\",\n    \"alpha\": {\n      \"b\": 2\n    },\n    \"status\": \"in-progress\"\n  }\n]"
	os.WriteFile(file, []byte(original), 0o644)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	RecordEnd(file, Entry{"sessionName": "old", "status": "completed", "exitCode": 0}, now)
	got, _ := os.ReadFile(file)
	want := "[\n  {\n    \"zeta\": 1,\n    \"sessionName\": \"old\",\n    \"alpha\": {\n      \"b\": 2\n    },\n    \"status\": \"completed\",\n    \"exitCode\": 0,\n    \"endedAt\": \"2026-10-03T12:00:00.000Z\"\n  }\n]"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// An untouched file round-trips byte for byte.
	os.WriteFile(file, []byte(original), 0o644)
	if err := WriteRows(file, ReadRows(file)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != original {
		t.Errorf("round trip changed the file:\n%s", got)
	}
}
