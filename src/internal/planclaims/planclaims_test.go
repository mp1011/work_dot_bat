package planclaims

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"work_dot_bat/src/internal/repo"
)

var t0 = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func testEnv(c *clock) Env {
	return Env{
		Now:            func() time.Time { return c.now },
		Hostname:       "HOST",
		PIDAlive:       func(int) bool { return true },
		StartTimeOf:    func(int) (time.Time, bool) { return time.Time{}, false },
		WorktreeExists: func(string) bool { return true },
		PID:            4242,
	}
}

func fixture(t *testing.T) (root string, c *clock, cli CLI, out, errb *bytes.Buffer) {
	t.Helper()
	root = t.TempDir()
	p := filepath.Join(root, "agent_docs", "plans", "x")
	os.MkdirAll(p, 0o755)
	os.WriteFile(filepath.Join(p, "plan.md"), []byte("---\nstatus: In Progress\n---\n\nparent\n"), 0o644)
	os.WriteFile(filepath.Join(p, "01.md"), []byte("---\nstatus: Not Started\nsubtask_of: plans/x/plan.md\n---\n\nchild\n"), 0o644)
	c = &clock{t0}
	out, errb = &bytes.Buffer{}, &bytes.Buffer{}
	cli = CLI{Root: root, Stdout: out, Stderr: errb, Env: testEnv(c)}
	return
}

func (c CLI) do(out, errb *bytes.Buffer, args ...string) (int, string, string) {
	out.Reset()
	errb.Reset()
	code := c.Run(args)
	return code, out.String(), errb.String()
}

const sub = "plans/x/01.md"

func TestSecondAcquireBlockedThenTTLRecovers(t *testing.T) {
	_, c, cli, out, errb := fixture(t)
	code, o, _ := cli.do(out, errb, "acquire", sub, "--owner", "me")
	if code != 0 || o != "ACQUIRED  plans/x/01.md  (branch x, ttl 4h)\n" {
		t.Fatalf("code=%d out=%q", code, o)
	}
	code, o, _ = cli.do(out, errb, "acquire", sub, "--owner", "other")
	want := "HELD  plans/x/01.md\n" +
		"      owner     agent \"me\" (pid 4242, HOST)\n" +
		"      branch    x\n" +
		"      worktree  (none)\n" +
		"      age       0s (ttl 4h — fresh)\n"
	if code != 1 || o != want {
		t.Fatalf("code=%d out=%q", code, o)
	}
	// Past TTL with the owner unprovable (agent pid is alive here): reported, not stolen.
	c.now = t0.Add(5 * time.Hour)
	code, o, _ = cli.do(out, errb, "acquire", sub, "--owner", "other")
	if code != 1 || !strings.Contains(o, "past ttl (5h) — owner may still be running; use --steal to override") {
		t.Fatalf("code=%d out=%q", code, o)
	}
	// Past TTL with the owner's pid provably dead: reclaimed.
	cli.Env.PIDAlive = func(int) bool { return false }
	if code, o, _ = cli.do(out, errb, "acquire", sub, "--owner", "other"); code != 0 {
		t.Fatalf("code=%d out=%q", code, o)
	}
	if _, o, _ = cli.do(out, errb, "list"); !strings.Contains(o, "other") && !strings.Contains(o, "agent") {
		t.Fatalf("out=%q", o)
	}
}

func TestCheckFreeHeldAndReadOnly(t *testing.T) {
	root, _, cli, out, errb := fixture(t)
	if code, o, _ := cli.do(out, errb, "check", sub); code != 0 || o != "FREE  plans/x/01.md\n" {
		t.Fatalf("code=%d out=%q", code, o)
	}
	if _, err := os.Stat(repo.ClaimsDir(root)); err == nil {
		t.Fatal("check created the claims dir")
	}
	cli.do(out, errb, "acquire", sub, "--owner", "me")
	code, o, _ := cli.do(out, errb, "check", sub)
	if code != 1 || strings.Count(o, "HELD  ") != 1 { // plan + branch scopes dedupe to one block
		t.Fatalf("code=%d out=%q", code, o)
	}
}

func TestConcurrentCheck(t *testing.T) {
	_, _, cli, out, errb := fixture(t)
	if code, o, _ := cli.do(out, errb, "concurrent-check", sub); code != 0 || o != "FREE  plans/x/01.md\n" {
		t.Fatalf("code=%d out=%q", code, o)
	}
	cli.do(out, errb, "acquire", sub, "--owner", "me")
	code, o, _ := cli.do(out, errb, "concurrent-check", sub)
	if code != 1 || !strings.HasPrefix(o, "BLOCKED  plans/x/01.md\n      live claim — held by agent \"me\" (pid 4242, HOST), 0s ago\n") {
		t.Fatalf("code=%d out=%q", code, o)
	}
}

func TestReleaseRenewAndErrors(t *testing.T) {
	_, c, cli, out, errb := fixture(t)
	if code, _, e := cli.do(out, errb, "renew", sub); code != 1 || !strings.HasPrefix(e, `Cannot renew plan claim for "plans/x/01.md": Cannot renew plan:plans/x/01.md — no claim held`) {
		t.Fatalf("code=%d err=%q", code, e)
	}
	cli.do(out, errb, "acquire", sub, "--owner", "me")
	c.now = t0.Add(time.Hour)
	if code, o, _ := cli.do(out, errb, "renew", sub); code != 0 || o != "RENEWED  plans/x/01.md\n" {
		t.Fatalf("code=%d out=%q", code, o)
	}
	cli.do(out, errb, "release", sub)
	if code, o, _ := cli.do(out, errb, "release", sub); code != 0 || o != "RELEASED  plans/x/01.md\n" {
		t.Fatalf("code=%d out=%q", code, o)
	}
	if _, o, _ := cli.do(out, errb, "list"); o != "No live claims.\n" {
		t.Fatalf("out=%q", o)
	}
	if code, _, e := cli.do(out, errb, "acquire", sub); code != 1 || !strings.HasPrefix(e, "Usage: plan-claim.sh acquire") {
		t.Fatalf("err=%q", e)
	}
	if _, _, e := cli.do(out, errb, "acquire", sub, "--owner", "x", "--kind", "bogus"); e != "Invalid --kind \"bogus\". Allowed: daemon, agent, launcher\n" {
		t.Fatalf("err=%q", e)
	}
	if _, _, e := cli.do(out, errb, "acquire", sub, "--owner", "x", "--pid", "abc"); e != "Invalid --pid \"abc\". Expected a positive integer.\n" {
		t.Fatalf("err=%q", e)
	}
	if code, _, e := cli.do(out, errb, "nope"); code != 1 || !strings.HasPrefix(e, "Unknown command: \"nope\"\nUsage: plan-claim.sh <check|") {
		t.Fatalf("err=%q", e)
	}
}

func TestStealPrintsOverriddenClaimFirst(t *testing.T) {
	_, _, cli, out, errb := fixture(t)
	cli.do(out, errb, "acquire", sub, "--owner", "me")
	code, o, _ := cli.do(out, errb, "acquire", sub, "--owner", "thief", "--steal")
	if code != 0 || !strings.HasPrefix(o, "OVERRIDING  plans/x/01.md\n      owner     agent \"me\"") || !strings.HasSuffix(o, "ACQUIRED  plans/x/01.md  (branch x, ttl 4h)\n") {
		t.Fatalf("code=%d out=%q", code, o)
	}
}

func TestPruneRemovesDeadLauncherAndMarksLedger(t *testing.T) {
	root, _, cli, out, errb := fixture(t)
	cli.do(out, errb, "acquire", sub, "--owner", "wk", "--kind", "launcher", "--pid", "777")
	cli.Env.PIDAlive = func(pid int) bool { return false }
	ledger := repo.LedgerPath(root)
	os.WriteFile(ledger, []byte(`[{"plan":"plans/x/01.md","status":"in-progress","pid":777,"extra":"keep"},{"plan":"plans/other.md","status":"in-progress"}]`), 0o644)
	code, o, _ := cli.do(out, errb, "prune")
	if code != 0 || !strings.Contains(o, "PRUNED  plan plans/x/01.md  — launcher \"wk\" (pid 777, HOST): owner pid 777 is not running on HOST\n") {
		t.Fatalf("code=%d out=%q", code, o)
	}
	b, _ := os.ReadFile(ledger)
	if !strings.Contains(string(b), `"status": "abandoned"`) || !strings.Contains(string(b), `"extra": "keep"`) || strings.Count(string(b), "in-progress") != 1 {
		t.Fatalf("ledger=%s", b)
	}
	if _, o, _ = cli.do(out, errb, "prune"); o != "Nothing to prune.\n" {
		t.Fatalf("out=%q", o)
	}
}

func TestPruneKeepsLiveAndNamesStaleReport(t *testing.T) {
	_, c, cli, out, errb := fixture(t)
	cli.do(out, errb, "acquire", sub, "--owner", "me")
	if _, o, _ := cli.do(out, errb, "prune"); o != "Nothing to prune.\n" {
		t.Fatalf("out=%q", o)
	}
	c.now = t0.Add(6 * time.Hour)
	_, o, _ := cli.do(out, errb, "prune")
	if !strings.Contains(o, "KEPT    plan plans/x/01.md  — past ttl (6h)") || !strings.HasSuffix(o, "Nothing to prune.\n") {
		t.Fatalf("out=%q", o)
	}
	if _, o, _ = cli.do(out, errb, "list"); !strings.Contains(o, "STALE — past ttl (6h)") {
		t.Fatalf("out=%q", o)
	}
}

func TestDeadDaemonReclaimedBeforeTTL(t *testing.T) {
	d := ClaimDecision(&Claim{Owner: &Owner{Kind: "daemon", PID: 9, Host: "H"}, AcquiredAt: formatTime(t0), TTLMs: 1800000}, t0.Add(time.Minute),
		Liveness{PIDAlive: boolp(false)})
	if d.State != StaleReclaimable {
		t.Fatalf("%+v", d)
	}
	a := ClaimDecision(&Claim{Owner: &Owner{Kind: "agent", PID: 9, Host: "H"}, AcquiredAt: formatTime(t0), TTLMs: 1800000}, t0.Add(time.Minute),
		Liveness{PIDAlive: boolp(false)})
	if a.State != Held {
		t.Fatalf("agent claim must stay TTL-bound: %+v", a)
	}
}

func TestReusedPidIsNotTheOwner(t *testing.T) {
	env := testEnv(&clock{t0})
	env.StartTimeOf = func(int) (time.Time, bool) { return t0.Add(time.Hour), true }
	c := &Claim{Owner: &Owner{Kind: "launcher", PID: 5, Host: "HOST", StartedAt: formatTime(t0)}, AcquiredAt: formatTime(t0), TTLMs: 1000}
	if lv := env.liveness(c); lv.PIDAlive == nil || *lv.PIDAlive {
		t.Fatalf("reused pid treated as alive: %+v", lv)
	}
	env.StartTimeOf = func(int) (time.Time, bool) { return t0.Add(time.Second), true }
	if lv := env.liveness(c); !*lv.PIDAlive {
		t.Fatal("same process within tolerance treated as dead")
	}
	c.Owner.Host = "elsewhere"
	if lv := env.liveness(c); lv.PIDAlive != nil {
		t.Fatal("foreign host must be undetermined")
	}
}

func TestNodeWrittenClaimIsReadAndGoFileLooksLikeNodes(t *testing.T) {
	root, _, cli, out, errb := fixture(t)
	nodeJSON := `{
  "scope": "plan",
  "id": "plans/x/01.md",
  "owner": {
    "kind": "agent",
    "label": "node-run",
    "pid": 31337,
    "host": "HOST"
  },
  "branch": "x",
  "worktree": null,
  "acquiredAt": "2026-10-03T12:00:00.000Z",
  "renewedAt": "2026-10-03T12:00:00.000Z",
  "ttlMs": 14400000
}
`
	dir := repo.ClaimsDir(root)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "plan--plans__x__01.md.json"), []byte(nodeJSON), 0o644)
	code, o, _ := cli.do(out, errb, "check", sub)
	if code != 1 || !strings.Contains(o, `owner     agent "node-run" (pid 31337, HOST)`) {
		t.Fatalf("code=%d out=%q", code, o)
	}
	// Go's own file is byte-identical in shape to Node's.
	cli.do(out, errb, "release", sub)
	cli.do(out, errb, "acquire", sub, "--owner", "node-run", "--pid", "31337")
	b, _ := os.ReadFile(filepath.Join(dir, "plan--plans__x__01.md.json"))
	if string(b) != nodeJSON {
		t.Fatalf("got\n%s\nwant\n%s", b, nodeJSON)
	}
}

func TestClaimFileNameAndAge(t *testing.T) {
	if a, b := ClaimFileName("plan", "plans/foo/bar.md"), ClaimFileName("plan", "plans/foo-bar.md"); a == b || a != "plan--plans__foo__bar.md.json" {
		t.Fatalf("%s %s", a, b)
	}
	for in, want := range map[time.Duration]string{59 * time.Second: "59s", 61 * time.Second: "1m", 3 * time.Hour: "3h", 50 * time.Hour: "2d"} {
		if got := FormatAge(in); got != want {
			t.Errorf("%v: %s want %s", in, got, want)
		}
	}
}
