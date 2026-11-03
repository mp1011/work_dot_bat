package quota

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseClaudeUsage(t *testing.T) {
	input := `You are currently using your subscription to power your Claude Code usage

Current session: 7% used · resets Oct 31, 1:40pm (America/New_York)
Current week (all models): 13% used · resets Nov 3, 7am (America/New_York)

What's contributing to your usage?
Approximate · sessions on this machine only · shares overlap`

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	models, err := ParseClaudeUsage(input, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Model != "Current session" || models[0].Percent != "93% remaining" {
		t.Errorf("model 0 mismatch: %+v", models[0])
	}
	if !strings.Contains(models[0].Refreshes, "Oct 31") || !strings.Contains(models[0].Refreshes, "1:40pm") {
		t.Errorf("model 0 refreshes unexpected: %s", models[0].Refreshes)
	}

	if models[1].Model != "Current week (all models)" || models[1].Percent != "87% remaining" {
		t.Errorf("model 1 mismatch: %+v", models[1])
	}
	if !strings.Contains(models[1].Refreshes, "Nov 3") || !strings.Contains(models[1].Refreshes, "7:00am") {
		t.Errorf("model 1 refreshes unexpected: %s", models[1].Refreshes)
	}
}

func TestParseAgyUsage(t *testing.T) {
	input := `Gemini Models	Weekly Limit Remaining	96%	2026-11-03T23:41:40Z
Gemini Models	Five Hour Limit Remaining	99%	2026-10-31T17:45:12Z
Claude and GPT models	Weekly Limit Remaining	87%	2026-11-03T12:52:27Z
Claude and GPT models	Five Hour Limit Remaining	85%	2026-10-31T17:49:35Z
Gemini 4 Argon	Gemini 4 Argon Weekly Limit	0%	2026-11-06T01:35:46Z`

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	models, err := ParseAgyUsage(input, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 5 {
		t.Fatalf("expected 5 models, got %d", len(models))
	}
	if models[0].Model != "Gemini Models (Weekly)" || models[0].Percent != "96% remaining" {
		t.Errorf("model 0 mismatch: %+v", models[0])
	}
	if models[1].Model != "Gemini Models (Five Hour)" || models[1].Percent != "99% remaining" {
		t.Errorf("model 1 mismatch: %+v", models[1])
	}
	if models[4].Model != "Gemini 4 Argon (Weekly)" || models[4].Percent != "0% remaining" {
		t.Errorf("model 4 mismatch: %+v", models[4])
	}
}

func TestParseCodexRateLimits(t *testing.T) {
	input := `{"id":2,"result":{"ordinaryUsageAllowed":true,"rateLimits":{"limitId":"codex","primary":{"usedPercent":0,"windowDurationMins":300,"resetsAt":1793469712},"secondary":{"usedPercent":24,"windowDurationMins":10080,"resetsAt":1793796101}}}}`

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	models, err := ParseCodexRateLimits(input, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Model != "Primary (5-Hour)" || models[0].Percent != "100% remaining" {
		t.Errorf("model 0 mismatch: %+v", models[0])
	}
	if models[1].Model != "Secondary (Weekly)" || models[1].Percent != "76% remaining" {
		t.Errorf("model 1 mismatch: %+v", models[1])
	}
}

func TestFormat(t *testing.T) {
	quotas := []AgentQuota{
		{
			Agent: "Claude",
			Models: []ModelQuota{
				{Model: "Current session", Percent: "93% remaining", Refreshes: "Oct 31, 1:40pm EDT"},
			},
		},
		{
			Agent: "AntiGravity",
			Models: []ModelQuota{
				{Model: "Gemini Models (Weekly)", Percent: "96% remaining", Refreshes: "Nov 03, 7:41pm EDT"},
			},
		},
		{
			Agent: "Codex",
			Error: errors.New("command not found"),
		},
	}

	out := Format(quotas)
	expected := `Claude
    Current session 93% remaining (Refreshes Oct 31, 1:40pm EDT)
AntiGravity
    Gemini Models (Weekly) 96% remaining (Refreshes Nov 03, 7:41pm EDT)
Codex
    Unavailable: command not found`

	if strings.TrimSpace(out) != strings.TrimSpace(expected) {
		t.Errorf("got:\n%s\nwant:\n%s", out, expected)
	}
}

func TestServiceFetch(t *testing.T) {
	svc := &Service{
		RunClaude: func(ctx context.Context) (string, error) {
			return "Current session: 5% used · resets Oct 31, 2:00pm (America/New_York)", nil
		},
		RunAgy: func(ctx context.Context) (string, error) {
			return "Gemini Models\tWeekly Limit Remaining\t90%\t2026-11-03T23:41:40Z", nil
		},
		RunCodex: func(ctx context.Context) (string, error) {
			return `{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":10,"windowDurationMins":300,"resetsAt":1793469712}}}}`, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
		},
	}

	all, err := svc.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("fetch all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(all))
	}

	filtered, err := svc.Fetch(context.Background(), "claude")
	if err != nil {
		t.Fatalf("fetch filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Agent != "Claude" {
		t.Fatalf("unexpected filtered: %+v", filtered)
	}

	agyFiltered, err := svc.Fetch(context.Background(), "agy")
	if err != nil {
		t.Fatalf("fetch agy filtered: %v", err)
	}
	if len(agyFiltered) != 1 || agyFiltered[0].Agent != "AntiGravity" {
		t.Fatalf("unexpected agy filtered: %+v", agyFiltered)
	}

	if _, err := svc.Fetch(context.Background(), "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent agent, got nil")
	}
}

func TestMalformedInputs(t *testing.T) {
	now := time.Now()
	if _, err := ParseClaudeUsage("no usage info here", now); err == nil {
		t.Errorf("expected error on empty claude output")
	}
	if _, err := ParseAgyUsage("invalid\tline", now); err == nil {
		t.Errorf("expected error on malformed agy output")
	}
	if _, err := ParseCodexRateLimits("{}", now); err == nil {
		t.Errorf("expected error on empty codex json")
	}
	if _, err := ParseCodexRateLimits("not json", now); err == nil {
		t.Errorf("expected error on invalid codex json")
	}
}

func TestRunOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"unknown_agent_xyz"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected code 1 on unknown agent, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown agent") {
		t.Errorf("expected stderr to mention unknown agent, got %q", stderr.String())
	}
}

func TestPercentColorAndColorize(t *testing.T) {
	cases := []struct {
		val      int
		wantCode string
	}{
		{100, "32"}, // Green (> 80)
		{81, "32"},  // Green (> 80)
		{80, "33"},  // Yellow (> 40, <= 80)
		{50, "33"},  // Yellow (> 40, <= 80)
		{41, "33"},  // Yellow (> 40, <= 80)
		{40, "31"},  // Red (<= 40)
		{15, "31"},  // Red (<= 40)
		{0, "31"},   // Red (<= 40)
	}

	for _, tc := range cases {
		got := PercentColor(tc.val)
		if got != tc.wantCode {
			t.Errorf("PercentColor(%d) = %q, want %q", tc.val, got, tc.wantCode)
		}
	}

	// Test ColorizePercent enabled
	green := ColorizePercent("90% remaining", true)
	if green != "\x1b[32m90% remaining\x1b[0m" {
		t.Errorf("expected green ANSI code, got %q", green)
	}

	yellow := ColorizePercent("76% remaining", true)
	if yellow != "\x1b[33m76% remaining\x1b[0m" {
		t.Errorf("expected yellow ANSI code, got %q", yellow)
	}

	red := ColorizePercent("0% remaining", true)
	if red != "\x1b[31m0% remaining\x1b[0m" {
		t.Errorf("expected red ANSI code, got %q", red)
	}

	// Test ColorizePercent disabled
	plain := ColorizePercent("90% remaining", false)
	if plain != "90% remaining" {
		t.Errorf("expected plain text when disabled, got %q", plain)
	}
}

func TestFormatColored(t *testing.T) {
	quotas := []AgentQuota{
		{
			Agent: "Claude",
			Models: []ModelQuota{
				{Model: "Current session", Percent: "90% remaining", Refreshes: "Oct 31, 1:40pm EDT"},
				{Model: "Current week", Percent: "30% remaining", Refreshes: "Nov 3, 7:00am EST"},
			},
		},
	}

	out := FormatColored(quotas, true)
	if !strings.Contains(out, "\x1b[32m90% remaining\x1b[0m") {
		t.Errorf("expected green percentage in formatted output, got %q", out)
	}
	if !strings.Contains(out, "\x1b[31m30% remaining\x1b[0m") {
		t.Errorf("expected red percentage in formatted output, got %q", out)
	}
}
