package monitor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"work_dot_bat/src/internal/agentsession"
	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
)

func TestFormatHeader(t *testing.T) {
	// Main branch omits Worktree
	if got := FormatHeader("music_mountain", ""); got != "music_mountain" {
		t.Errorf("main branch header = %q, want %q", got, "music_mountain")
	}
	// Worktree branch includes Project : Worktree
	if got := FormatHeader("music_mountain", "feature-audio"); got != "music_mountain : feature-audio" {
		t.Errorf("worktree branch header = %q, want %q", got, "music_mountain : feature-audio")
	}
}

func TestFormatItem(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		colorize bool
		want     string
	}{
		{
			name:     "Active with agent no color",
			item:     Item{Status: "Active", PlanPath: "plans/core/task.md", Agent: "agy"},
			colorize: false,
			want:     "    [Active] plans/core/task.md - agy",
		},
		{
			name:     "Stalled without agent no color",
			item:     Item{Status: "Stalled", PlanPath: "plans/core/task.md"},
			colorize: false,
			want:     "    [Stalled] plans/core/task.md",
		},
		{
			name:     "Blocked with agent no color",
			item:     Item{Status: "Blocked", PlanPath: "plans/err.md", Agent: "claude"},
			colorize: false,
			want:     "    [Blocked] plans/err.md - claude",
		},
		{
			name:     "Ready for Review with agent no color",
			item:     Item{Status: "Ready for Review", PlanPath: "plans/ready.md", Agent: "codex"},
			colorize: false,
			want:     "    [Ready for Review] plans/ready.md - codex",
		},
		{
			name:     "Active with color",
			item:     Item{Status: "Active", PlanPath: "plans/p.md", Agent: "agy"},
			colorize: true,
			want:     "    \x1b[36m[Active]\x1b[0m plans/p.md - agy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatItem(tt.item, tt.colorize)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatReport(t *testing.T) {
	groups := []Group{
		{
			Project:  "proj_a",
			Worktree: "", // main branch
			Items: []Item{
				{Status: "Active", PlanPath: "plans/a.md", Agent: "agy"},
				{Status: "Stalled", PlanPath: "plans/b.md"},
			},
		},
		{
			Project:  "proj_a",
			Worktree: "feat-x",
			Items: []Item{
				{Status: "Blocked", PlanPath: "plans/x.md", Agent: "claude"},
			},
		},
		{
			Project:  "proj_empty",
			Worktree: "",
			Items:    nil, // Should be omitted
		},
	}

	want := "proj_a\n" +
		"    [Active] plans/a.md - agy\n" +
		"    [Stalled] plans/b.md\n" +
		"\n" +
		"proj_a : feat-x\n" +
		"    [Blocked] plans/x.md - claude\n"

	got := FormatReport(groups, false)
	if got != want {
		t.Errorf("FormatReport mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFindActiveProcess(t *testing.T) {
	procs := []ProcessInfo{
		{PID: 101, CommandLine: `agy.exe --add-dir C:/repos/proj --model gemini -i "/do-work-yolo agent_docs/plans/feature/plan.md"`},
		{PID: 102, CommandLine: `cmd.exe /c codex --model gpt exec "/do-work-yolo agent_docs\plans\another\task.md"`},
		{PID: 103, CommandLine: `claude --model sonnet "Read agent_docs/plans/claude_task.md"`},
		{PID: 104, CommandLine: `notepad.exe somefile.txt`},
	}

	// 1. Matches agy
	active, agent := FindActiveProcess(procs, "plans/feature/plan.md", "C:/repos/proj", "")
	if !active || agent != "agy" {
		t.Errorf("plans/feature/plan.md: active=%v, agent=%q", active, agent)
	}

	// 2. Matches codex with backslashes in cmd
	active, agent = FindActiveProcess(procs, "plans/another/task.md", "C:/repos/proj", "")
	if !active || agent != "codex" {
		t.Errorf("plans/another/task.md: active=%v, agent=%q", active, agent)
	}

	// 3. Matches claude
	active, agent = FindActiveProcess(procs, "plans/claude_task.md", "C:/repos/proj", "")
	if !active || agent != "claude" {
		t.Errorf("plans/claude_task.md: active=%v, agent=%q", active, agent)
	}

	// 4. No match
	active, agent = FindActiveProcess(procs, "plans/unrelated.md", "C:/repos/proj", "")
	if active {
		t.Errorf("plans/unrelated.md should not be active")
	}
}

func writePlanFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, "agent_docs", filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectProjectGroups(t *testing.T) {
	repoDir := t.TempDir()

	// Write plans with different statuses
	writePlanFile(t, repoDir, "plans/active_plan.md", "---\nstatus: In Progress\n---\n# GOAL\nactive")
	writePlanFile(t, repoDir, "plans/stalled_plan.md", "---\nstatus: In Progress\n---\n# GOAL\nstalled")
	writePlanFile(t, repoDir, "plans/blocked_plan.md", "---\nstatus: Blocked - User Input Required\n---\n# GOAL\nblocked")
	writePlanFile(t, repoDir, "plans/ready_plan.md", "---\nstatus: Ready for Review\n---\n# GOAL\nready")
	writePlanFile(t, repoDir, "plans/done_plan.md", "---\nstatus: Done\n---\n# GOAL\ndone")
	writePlanFile(t, repoDir, "plans/not_started.md", "---\nstatus: Not Started\n---\n# GOAL\nnot started")

	// Add ledger record for stalled_plan to test ledger agent lookup
	ledgerFile := repo.LedgerPath(repoDir)
	_ = os.MkdirAll(filepath.Dir(ledgerFile), 0o755)
	_, _ = agentsession.RecordStart(ledgerFile, agentsession.Entry{
		"plan":      "agent_docs/plans/stalled_plan.md",
		"agent":     "claude",
		"sessionId": "test-sid",
	}, 1234, time.Now())

	procs := []ProcessInfo{
		{
			PID:         999,
			CommandLine: `agy --add-dir ` + repoDir + ` -i "/do-work-yolo agent_docs/plans/active_plan.md"`,
		},
	}

	p := projects.Project{
		Name: "test_proj",
		Path: repoDir,
	}

	groups := CollectProjectGroups(context.Background(), p, procs, nil)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group (main branch), got %d", len(groups))
	}
	g := groups[0]
	if g.Project != "test_proj" || g.Worktree != "" {
		t.Errorf("group header mismatch: proj=%q wt=%q", g.Project, g.Worktree)
	}

	if len(g.Items) != 4 {
		t.Fatalf("expected 4 items (Active, Stalled, Blocked, Ready for Review), got %d: %+v", len(g.Items), g.Items)
	}

	// Verify items:
	itemMap := map[string]Item{}
	for _, it := range g.Items {
		itemMap[it.PlanPath] = it
	}

	// 1. active_plan
	if it, ok := itemMap["plans/active_plan.md"]; !ok || it.Status != "Active" || it.Agent != "agy" {
		t.Errorf("active_plan: %+v", it)
	}

	// 2. stalled_plan (In Progress without active process -> Stalled, agent looked up from ledger)
	if it, ok := itemMap["plans/stalled_plan.md"]; !ok || it.Status != "Stalled" || it.Agent != "claude" {
		t.Errorf("stalled_plan: %+v", it)
	}

	// 3. blocked_plan
	if it, ok := itemMap["plans/blocked_plan.md"]; !ok || it.Status != "Blocked" {
		t.Errorf("blocked_plan: %+v", it)
	}

	// 4. ready_plan
	if it, ok := itemMap["plans/ready_plan.md"]; !ok || it.Status != "Ready for Review" {
		t.Errorf("ready_plan: %+v", it)
	}

	// 5. done_plan and not_started must NOT be present
	if _, ok := itemMap["plans/done_plan.md"]; ok {
		t.Errorf("done_plan should not be present")
	}
	if _, ok := itemMap["plans/not_started.md"]; ok {
		t.Errorf("not_started should not be present")
	}
}

func TestCollectProjectGroupsWithWorktrees(t *testing.T) {
	repoDir := t.TempDir()
	wtDir := t.TempDir()

	writePlanFile(t, repoDir, "plans/main_task.md", "---\nstatus: Ready for Review\n---\n# GOAL\nmain")
	writePlanFile(t, wtDir, "plans/wt_task.md", "---\nstatus: In Progress\n---\n# GOAL\nwt")

	collectWT := func(root string) (map[string]planstatus.WorktreeStatus, error) {
		return map[string]planstatus.WorktreeStatus{
			"plans/wt_task.md": {
				Status:   "In Progress",
				Branch:   "feature-branch",
				Worktree: wtDir,
			},
		}, nil
	}

	procs := []ProcessInfo{
		{
			PID:         888,
			CommandLine: `codex --model gpt exec "/do-work-yolo agent_docs/plans/wt_task.md"`,
		},
	}

	p := projects.Project{
		Name: "multi_proj",
		Path: repoDir,
	}

	groups := CollectProjectGroups(context.Background(), p, procs, collectWT)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (main and feature-branch), got %d: %+v", len(groups), groups)
	}

	// Group 0: main
	if groups[0].Worktree != "" || len(groups[0].Items) != 1 || groups[0].Items[0].PlanPath != "plans/main_task.md" {
		t.Errorf("group 0 mismatch: %+v", groups[0])
	}

	// Group 1: feature-branch
	if groups[1].Worktree != "feature-branch" || len(groups[1].Items) != 1 {
		t.Fatalf("group 1 mismatch: %+v", groups[1])
	}
	wtItem := groups[1].Items[0]
	if wtItem.Status != "Active" || wtItem.Agent != "codex" || wtItem.PlanPath != "plans/wt_task.md" {
		t.Errorf("wt item mismatch: %+v", wtItem)
	}
}

func TestRunCommand(t *testing.T) {
	repoDir := t.TempDir()
	writePlanFile(t, repoDir, "plans/p.md", "---\nstatus: Blocked\n---\n# GOAL\np")

	var stdout, stderr bytes.Buffer
	cfg := Config{
		Projects: []projects.Project{{Name: "alpha", Path: repoDir}},
		ProcessLister: func(ctx context.Context) ([]ProcessInfo, error) {
			return nil, nil
		},
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Run(cfg)
	if code != 0 {
		t.Fatalf("Run returned %d, stderr: %s", code, stderr.String())
	}

	want := "alpha\n    [Blocked] plans/p.md\n"
	if stdout.String() != want {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", stdout.String(), want)
	}
}

func TestDefaultProcessLister(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	procs, err := DefaultProcessLister(ctx)
	if err != nil {
		t.Fatalf("DefaultProcessLister failed: %v", err)
	}
	if len(procs) == 0 {
		t.Logf("Warning: 0 processes returned by DefaultProcessLister")
	}
}

