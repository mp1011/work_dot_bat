package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"work_dot_bat/src/internal/proc"

	"work_dot_bat/src/internal/planstatus"
)

func plan(path, status string, mods ...func(*Plan)) Plan {
	p := Plan{Entry: planstatus.Entry{Path: path, Status: status}, Source: "main"}
	for _, m := range mods {
		m(&p)
	}
	return p
}

func auto(p *Plan)                  { p.Auto = true }
func sub(parent string) func(*Plan) { return func(p *Plan) { p.SubtaskOf = parent } }
func dep(d string) func(*Plan)      { return func(p *Plan) { p.DependsOn = d } }

func paths(ps []Plan) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Path)
	}
	return out
}

func TestEligibilityOrderingAndReasons(t *testing.T) {
	plans := []Plan{
		plan("plans/a_manual.md", "Not Started"),
		plan("plans/b_ns.md", "Not Started", auto),
		plan("plans/c_ip.md", "In Progress", auto),
		plan("plans/d_done.md", "Done", auto),
		plan("plans/e_blocked.md", "Blocked - User Input Required", auto, func(p *Plan) { p.BlockedReason = "need a key" }),
		plan("plans/p/plan.md", "In Progress", auto),
		plan("plans/p/01.md", "Ready for Review", sub("plans/p/plan.md")),
		plan("plans/p/02.md", "Not Started", sub("plans/p/plan.md")),
		plan("plans/p/03.md", "Not Started", sub("plans/p/plan.md")),
		plan("plans/f_dep.md", "Not Started", auto, dep("plans/b_ns.md")),
		plan("plans/g_uncommitted.md", "Not Started", auto),
	}
	res := Eligibility(plans, Inputs{
		Committed: func(p Plan) bool { return p.Path != "plans/g_uncommitted.md" },
	})
	// In Progress first, then Not Started, each by path.
	if got := paths(res.Eligible); !reflect.DeepEqual(got, []string{"plans/c_ip.md", "plans/b_ns.md", "plans/p/02.md"}) {
		t.Errorf("eligible %v", got)
	}
	if got := paths(res.Blocked); !reflect.DeepEqual(got, []string{"plans/e_blocked.md"}) {
		t.Errorf("blocked %v", got)
	}
	want := map[string]string{
		"plans/a_manual.md":      "not marked auto: true",
		"plans/d_done.md":        "status is Done",
		"plans/p/plan.md":        "has subtasks — work the subtasks instead",
		"plans/p/01.md":          "status is Ready for Review",
		"plans/p/03.md":          "earlier subtask plans/p/02.md (Not Started)",
		"plans/f_dep.md":         "depends_on plans/b_ns.md (Not Started)",
		"plans/g_uncommitted.md": "uncommitted changes in plan file",
	}
	for p, w := range want {
		if res.Reasons[p] != w {
			t.Errorf("reason for %s: %q, want %q", p, res.Reasons[p], w)
		}
	}
	if _, ok := res.Reasons["plans/c_ip.md"]; ok {
		t.Error("eligible plans have no reason")
	}
}

func TestEligibilityMissingDependency(t *testing.T) {
	res := Eligibility([]Plan{plan("plans/x.md", "Not Started", auto, dep("plans/gone.md"))}, Inputs{})
	if res.Reasons["plans/x.md"] != "depends_on plans/gone.md (missing)" || len(res.Eligible) != 0 {
		t.Errorf("%+v", res)
	}
}

func TestDaemonBranchForUsesTopAutoAncestor(t *testing.T) {
	plans := []Plan{
		plan("plans/top/plan.md", "In Progress", auto),
		plan("plans/top/mid.md", "In Progress", auto, sub("plans/top/plan.md")),
		plan("plans/top/leaf.md", "Not Started", sub("plans/top/mid.md")),
		plan("plans/solo/plan.md", "In Progress"),
		plan("plans/solo/leaf.md", "Not Started", auto, sub("plans/solo/plan.md")),
	}
	top := planstatus.SlugForPlan("plans/top/plan.md")
	if got := DaemonBranchFor("plans/top/leaf.md", plans); got != top {
		t.Errorf("leaf %q want %q", got, top)
	}
	if got := DaemonBranchFor("plans/top/mid.md", plans); got != top {
		t.Errorf("mid %q", got)
	}
	// The parent isn't auto, so the leaf (its own auto root) gets its own branch.
	if got := DaemonBranchFor("plans/solo/leaf.md", plans); got != planstatus.SlugForPlan("plans/solo/leaf.md") {
		t.Errorf("solo %q", got)
	}
	if !IsAutoCandidate(plans[2], plans) || IsAutoCandidate(plans[3], plans) {
		t.Error("auto ancestry")
	}
}

func TestAutoStatusReportGolden(t *testing.T) {
	plans := []Plan{
		plan("plans/a.md", "Not Started", auto),
		plan("plans/bb.md", "In Progress", auto, func(p *Plan) { p.Source, p.WorktreeBranch = "C:/r/proj-bb", "bb" }),
		plan("plans/c.md", "Blocked - User Input Required", auto, func(p *Plan) { p.BlockedReason = "need creds" }),
		plan("plans/d.md", "Done", auto),
		plan("plans/zz.md", "Not Started"),
	}
	res := Eligibility(plans, Inputs{})
	want := "BLOCKED — daemon will not proceed (1):\n" +
		"  plans/c.md   [main]             need creds\n\n" +
		"ELIGIBLE (next first):\n" +
		"  plans/bb.md   [worktree bb]   In Progress\n" +
		"  plans/a.md    [main]          Not Started\n\n" +
		"NOT ELIGIBLE (1):\n" +
		"  plans/d.md   [main]   status is Done"
	_ = want
	got := FormatAutoStatusReport(res, plans)
	for _, line := range []string{
		"BLOCKED — daemon will not proceed (1):",
		"need creds",
		"ELIGIBLE (next first):",
		"[worktree bb]",
		"NOT ELIGIBLE (1):",
		"status is Done",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("report missing %q:\n%s", line, got)
		}
	}
	// Columns line up: every detail starts at the same offset within its section.
	var offsets []int
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "  plans/") {
			offsets = append(offsets, strings.Index(l, "[")) // source column
		}
	}
	for _, o := range offsets {
		if o != offsets[0] {
			t.Errorf("source column misaligned: %v\n%s", offsets, got)
			break
		}
	}
	if FormatAutoStatusReport(Eligibility([]Plan{plan("plans/x.md", "Not Started")}, Inputs{}), []Plan{plan("plans/x.md", "Not Started")}) != `No plans are marked "auto: true".` {
		t.Error("empty report")
	}
}

func TestParseOutcome(t *testing.T) {
	cases := []struct {
		in   string
		want *Outcome
	}{
		{"all good\nAUTO_OUTCOME: done", &Outcome{Kind: "done"}},
		{"AUTO_OUTCOME: subtasks\n", &Outcome{Kind: "subtasks"}},
		{"AUTO_OUTCOME: blocked — need the API key", &Outcome{Kind: "blocked", Reason: "need the API key"}},
		{"AUTO_OUTCOME: blocked - dash reason", &Outcome{Kind: "blocked", Reason: "dash reason"}},
		{"AUTO_OUTCOME: blocked", &Outcome{Kind: "blocked"}},
		{"**AUTO_OUTCOME: done**", &Outcome{Kind: "done"}},
		{"`AUTO_OUTCOME: done`", &Outcome{Kind: "done"}},
		{"AUTO_OUTCOME: blocked — x\nAUTO_OUTCOME: done", &Outcome{Kind: "done"}}, // the last marker wins
		{"auto_outcome: DONE", &Outcome{Kind: "done"}},
		{"AUTO_OUTCOME: finished", nil},
		{"no marker here", nil},
		{"", nil},
		{"I will end with AUTO_OUTCOME: done soon", nil}, // not a line of its own
	}
	for _, c := range cases {
		if got := ParseOutcome(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %+v want %+v", c.in, got, c.want)
		}
	}
}

func TestParseAssessment(t *testing.T) {
	if a := ParseAssessment("looks fine\nASSESSMENT: complete"); a == nil || !a.Complete {
		t.Errorf("%+v", a)
	}
	a := ParseAssessment("Gaps:\n\n- no tests for X\n- docs missing\n1. numbered gap\n\nASSESSMENT: incomplete")
	if a == nil || a.Complete || !reflect.DeepEqual(a.Gaps, []string{"no tests for X", "docs missing", "numbered gap"}) {
		t.Errorf("%+v", a)
	}
	if a := ParseAssessment("ASSESSMENT: incomplete"); a == nil || a.Complete || len(a.Gaps) != 0 {
		t.Errorf("no bullets: %+v", a)
	}
	if a := ParseAssessment("ASSESSMENT: incomplete\n...\nASSESSMENT: complete"); a == nil || !a.Complete {
		t.Errorf("last wins: %+v", a)
	}
	if ParseAssessment("nothing") != nil {
		t.Error("no marker")
	}
}

func TestPrompts(t *testing.T) {
	work := BuildDaemonWorkPrompt("agent_docs/plans/x.md", "doc.md")
	want := "/do-work-yolo agent_docs/plans/x.md\n\nDAEMON MODE\nThe daemon manages branches, git commits, and review status.\n" +
		"Skip Step Zero entirely. Do not run plan-status.sh.\nDo not commit or push.\n" +
		"Follow the DAEMON MODE instructions in doc.md.\nFinish with exactly one final line:\nAUTO_OUTCOME: done\nAUTO_OUTCOME: subtasks\nAUTO_OUTCOME: blocked — <one-line reason>"
	if work != want {
		t.Errorf("%q", work)
	}
	f := BuildDaemonFollowupPrompt("p.md", Problem{Failures: []string{"lint", "unit"}}, 2, 3)
	if !strings.HasPrefix(f, "Follow-up on p.md (attempt 2 of 3).\n\nDAEMON MODE\n\nAutomated test/check failures:\n- lint\n- unit\n") ||
		!strings.HasSuffix(f, "AUTO_OUTCOME: done | subtasks | blocked — <reason>") {
		t.Errorf("%q", f)
	}
	if g := BuildDaemonFollowupPrompt("p.md", Problem{Gaps: []string{"g1"}}, 2, 3); !strings.Contains(g, "Assessment identified incomplete requirements (gaps):\n- g1") {
		t.Error(g)
	}
	if n := BuildDaemonFollowupPrompt("p.md", Problem{NoChanges: true}, 2, 3); !strings.Contains(n, "completed without producing any code changes") {
		t.Error(n)
	}
	if m := BuildDaemonFollowupPrompt("p.md", Problem{Message: "Missing outcome marker"}, 2, 3); !strings.Contains(m, "Problem: Missing outcome marker") {
		t.Error(m)
	}
	c := BuildDaemonContinuationPrompt("p.md", "doc.md", Problem{}, 1, 3, "limit")
	if !strings.HasPrefix(c, "/do-work-yolo p.md\n\nDAEMON MODE\nYou were interrupted by a usage limit; the uncommitted changes") {
		t.Errorf("%q", c)
	}
	c = BuildDaemonContinuationPrompt("p.md", "doc.md", Problem{}, 2, 3, "interrupted")
	if !strings.HasPrefix(c, "Follow-up on p.md (attempt 2 of 3).\n\nDAEMON MODE\nYou are resuming an earlier session that was interrupted;") {
		t.Errorf("%q", c)
	}
	if BuildAssessPrompt("p.md") != "/assess-done p.md" {
		t.Error("assess prompt")
	}
}

func TestCommittedPlanPaths(t *testing.T) {
	calls := map[string]int{}
	git := func(dir string, args ...string) (string, error) {
		calls[dir]++
		switch args[0] {
		case "ls-files":
			return "agent_docs/plans/a.md\nagent_docs/plans/b.md\nagent_docs/plans/c.md\n", nil
		case "status":
			return " M agent_docs/plans/b.md\n?? agent_docs/plans/new.md\n", nil
		}
		return "", nil
	}
	plans := []Plan{plan("plans/a.md", "Not Started"), plan("plans/b.md", "Not Started"), plan("plans/new.md", "Not Started")}
	got := CommittedPlanPaths("/main", plans, git)
	if !got["plans/a.md"] || got["plans/b.md"] || got["plans/new.md"] {
		t.Errorf("%v", got)
	}
	if calls["/main"] != 2 {
		t.Errorf("two git calls per root, got %d", calls["/main"])
	}

	// In a worktree a dirty In Progress plan counts (the daemon wrote it), and so does one whose
	// diff is only the status line.
	git2 := func(dir string, args ...string) (string, error) {
		switch args[0] {
		case "ls-files":
			return "agent_docs/plans/a.md\nagent_docs/plans/b.md\nagent_docs/plans/c.md\n", nil
		case "status":
			return " M agent_docs/plans/a.md\n M agent_docs/plans/b.md\n M agent_docs/plans/c.md\n", nil
		case "diff":
			if args[len(args)-1] == "agent_docs/plans/b.md" {
				return "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-status: Not Started\n+status: In Progress\n", nil
			}
			return "-old line\n+new line\n", nil
		}
		return "", nil
	}
	wt := func(path, status string) Plan {
		p := plan(path, status)
		p.Source = "/wt"
		return p
	}
	got = CommittedPlanPaths("/main", []Plan{wt("plans/a.md", "In Progress"), wt("plans/b.md", "Not Started"), wt("plans/c.md", "Not Started")}, git2)
	if !got["plans/a.md"] || !got["plans/b.md"] || got["plans/c.md"] {
		t.Errorf("%v", got)
	}
}

func TestIsDiffOnlyStatusChange(t *testing.T) {
	if !IsDiffOnlyStatusChange("--- a\n+++ b\n@@ -1 +1 @@\n-status: Not Started\n+status: Done\n") {
		t.Error("status-only")
	}
	if IsDiffOnlyStatusChange("-status: a\n+status: b\n+extra line\n") || IsDiffOnlyStatusChange("") {
		t.Error("not status-only")
	}
}

func TestExtractBlockedReason(t *testing.T) {
	if got := ExtractBlockedReason("# GOAL\nx\n\n## Blocked — Needs User Input\n\nneed a key\nmore\n\n## Next\ny\n"); got != "need a key" {
		t.Errorf("%q", got)
	}
	if ExtractBlockedReason("no blocked section") != "" || ExtractBlockedReason("") != "" {
		t.Error("none")
	}
}

func TestReadPlanStatesPrefersWorktreeCopy(t *testing.T) {
	main := t.TempDir()
	wt := t.TempDir()
	write := func(root, rel, content string) {
		full := filepath.Join(root, "agent_docs", filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
	write(main, "plans/x.md", "---\nstatus: In Progress\nauto: true\n---\nx\n")
	write(wt, "plans/x.md", "---\nstatus: Blocked - User Input Required\nauto: true\n---\n\n## Blocked — Needs User Input\nneed a key\n")
	write(wt, "plans/only_in_wt.md", "---\nstatus: Not Started\n---\nx\n")
	slug := planstatus.SlugForPlan("plans/x.md")
	got := ReadPlanStates(main, []planstatus.Worktree{
		{Path: main, Branch: "main"},
		{Path: wt, Branch: slug},
		{Path: filepath.Join(t.TempDir(), "gone"), Branch: "gone"},
		{Path: wt, Branch: "pruned", Prunable: true},
	}, nil)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	x := got[1]
	if x.Path != "plans/x.md" || x.Status != "Blocked - User Input Required" || x.Source != wt || x.BlockedReason != "need a key" || x.WorktreeBranch != slug {
		t.Errorf("%+v", x)
	}
	if got[0].Path != "plans/only_in_wt.md" || got[0].Source != wt {
		t.Errorf("%+v", got[0])
	}
}

// --- process-tree kill ---

func TestHelperProc(t *testing.T) {
	switch os.Getenv("WDB_HELPER") {
	case "parent":
		// Spawn a grandchild that outlives us unless the whole tree is killed.
		c := exec.Command(os.Args[0], "-test.run=^TestHelperProc$")
		c.Env = append(os.Environ(), "WDB_HELPER=child")
		c.Start()
		os.WriteFile(os.Getenv("WDB_PIDFILE"), []byte(itoa(c.Process.Pid)), 0o644)
		time.Sleep(time.Minute)
	case "child":
		time.Sleep(time.Minute)
	case "exit3":
		os.Exit(3)
	}
}

func processAlive(pid int) bool {
	if os.PathSeparator == '\\' {
		out, _ := exec.Command("tasklist", "/FI", "PID eq "+itoa(pid), "/NH").Output()
		return strings.Contains(string(out), " "+itoa(pid)+" ")
	}
	p, _ := os.FindProcess(pid)
	return p.Signal(nil) == nil
}

func TestRunProcessExitCode(t *testing.T) {
	t.Setenv("WDB_HELPER", "exit3")
	code, err := proc.Run(context.Background(), os.Args[0], []string{"-test.run=^TestHelperProc$"}, proc.Options{})
	if code != 3 || err != nil {
		t.Errorf("code %d err %v", code, err)
	}
	if code, err := proc.Run(context.Background(), "definitely-not-a-binary-xyz", nil, proc.Options{}); code != 1 || err == nil {
		t.Errorf("missing binary: %d %v", code, err)
	}
}

func waitForFile(t *testing.T, p string) int {
	t.Helper()
	for i := 0; i < 200; i++ {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			n := 0
			for _, c := range string(b) {
				n = n*10 + int(c-'0')
			}
			return n
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("grandchild pid file never appeared")
	return 0
}

func TestCancelKillsTheWholeProcessTree(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("WDB_HELPER", "parent")
	t.Setenv("WDB_PIDFILE", pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		code, _ := proc.Run(ctx, os.Args[0], []string{"-test.run=^TestHelperProc$"}, proc.Options{})
		done <- code
	}()
	child := waitForFile(t, pidFile)
	if !processAlive(child) {
		t.Fatal("grandchild should be running before the cancel")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("RunProcess did not return after cancel")
	}
	for i := 0; i < 100 && processAlive(child); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if processAlive(child) {
		t.Error("the grandchild survived: orphaned agent process")
	}
}

func TestTimeoutKillsAndReports(t *testing.T) {
	t.Setenv("WDB_HELPER", "child")
	start := time.Now()
	code, err := proc.Run(context.Background(), os.Args[0], []string{"-test.run=^TestHelperProc$"}, proc.Options{Timeout: 500 * time.Millisecond})
	if err != proc.ErrTimedOut || code == 0 || time.Since(start) > 30*time.Second {
		t.Errorf("code %d err %v after %v", code, err, time.Since(start))
	}
}

func TestHeartbeatFiresWhileRunning(t *testing.T) {
	t.Setenv("WDB_HELPER", "child")
	beats := 0
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	proc.Run(ctx, os.Args[0], []string{"-test.run=^TestHelperProc$"}, proc.Options{
		Heartbeat: 100 * time.Millisecond, OnHeartbeat: func(time.Duration) { beats++ },
	})
	if beats < 2 {
		t.Errorf("heartbeats %d", beats)
	}
}
