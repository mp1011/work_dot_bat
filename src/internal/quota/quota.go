// Package quota collects and formats quota and rate limit status across AI coding agents
// (Claude Code, Google AntiGravity, OpenAI Codex).
package quota

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	// Embedded timezone database for Windows
	_ "time/tzdata"
)

// ModelQuota represents a quota window for a specific model or model family.
type ModelQuota struct {
	Model     string
	Percent   string // e.g. "93% remaining"
	Refreshes string // e.g. "Oct 31, 1:40pm EDT"
}

// AgentQuota represents the collection of quotas for one agent.
type AgentQuota struct {
	Agent  string
	Error  error
	Models []ModelQuota
}

var (
	claudeLineRE = regexp.MustCompile(`(?i)(Current session|Current week.*?):\s*(\d+)%\s*used.*?resets\s+([^\r\n]+)`)
	claudeDateRE = regexp.MustCompile(`(?i)([A-Za-z]+)\s+(\d{1,2}),?\s*(\d{1,2})(?::(\d{2}))?\s*(am|pm)\s*\(([^)]+)\)`)
)

func formatDate(t time.Time) string {
	return fmt.Sprintf("%s %d, %s", t.Format("Jan"), t.Day(), t.Format("3:04pm MST"))
}

// ParseClaudeUsage parses the stdout from `claude --print "/usage"`.
func ParseClaudeUsage(output string, now time.Time) ([]ModelQuota, error) {
	var models []ModelQuota
	matches := claudeLineRE.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		used, _ := strconv.Atoi(m[2])
		remaining := 100 - used
		if remaining < 0 {
			remaining = 0
		}
		dateStr := strings.TrimSpace(m[3])

		refreshes := dateStr
		if dm := claudeDateRE.FindStringSubmatch(dateStr); dm != nil {
			loc, err := time.LoadLocation(dm[6])
			if err == nil {
				parsedMonth, err := time.Parse("Jan", dm[1])
				if err == nil {
					day, _ := strconv.Atoi(dm[2])
					hour, _ := strconv.Atoi(dm[3])
					hour %= 12
					if strings.EqualFold(dm[5], "pm") {
						hour += 12
					}
					minute := 0
					if dm[4] != "" {
						minute, _ = strconv.Atoi(dm[4])
					}
					localNow := now.In(loc)
					year := localNow.Year()
					if parsedMonth.Month() < localNow.Month() && localNow.Month() == time.December {
						year++
					}
					parsedTime := time.Date(year, parsedMonth.Month(), day, hour, minute, 0, 0, loc)
					refreshes = formatDate(parsedTime)
				}
			}
		}

		models = append(models, ModelQuota{
			Model:     name,
			Percent:   fmt.Sprintf("%d%% remaining", remaining),
			Refreshes: refreshes,
		})
	}
	if len(models) == 0 {
		return nil, errors.New("no usage data found in Claude output")
	}
	return models, nil
}

// ParseAgyUsage parses the stdout from `agy --print "/usage"`.
func ParseAgyUsage(output string, now time.Time) ([]ModelQuota, error) {
	var models []ModelQuota
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		family := strings.TrimSpace(parts[0])
		limitDesc := strings.TrimSpace(parts[1])
		percentStr := strings.TrimSpace(parts[2])
		rawDate := strings.TrimSpace(parts[3])

		window := limitDesc
		if strings.Contains(limitDesc, "Five Hour") {
			window = "Five Hour"
		} else if strings.Contains(limitDesc, "Weekly") {
			window = "Weekly"
		}

		modelName := family
		if !strings.Contains(family, window) {
			modelName = fmt.Sprintf("%s (%s)", family, window)
		}

		percent := percentStr
		if !strings.HasSuffix(percent, "remaining") && !strings.HasSuffix(percent, "used") {
			percent = percent + " remaining"
		}

		refreshes := rawDate
		if t, err := time.Parse(time.RFC3339, rawDate); err == nil {
			refreshes = formatDate(t.Local())
		}

		models = append(models, ModelQuota{
			Model:     modelName,
			Percent:   percent,
			Refreshes: refreshes,
		})
	}
	if len(models) == 0 {
		return nil, errors.New("no usage data found in AntiGravity output")
	}
	return models, nil
}

type codexRateLimitWindow struct {
	UsedPercent        int   `json:"usedPercent"`
	WindowDurationMins int   `json:"windowDurationMins"`
	ResetsAt           int64 `json:"resetsAt"`
}

type codexRateLimit struct {
	Primary   *codexRateLimitWindow `json:"primary"`
	Secondary *codexRateLimitWindow `json:"secondary"`
}

type codexResponse struct {
	Result struct {
		RateLimits *codexRateLimit `json:"rateLimits"`
	} `json:"result"`
	RateLimits *codexRateLimit `json:"rateLimits"`
}

// ParseCodexRateLimits parses the JSON-RPC response from codex `account/rateLimits/read`.
func ParseCodexRateLimits(jsonStr string, now time.Time) ([]ModelQuota, error) {
	var resp codexResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("invalid codex JSON: %w", err)
	}
	rl := resp.Result.RateLimits
	if rl == nil {
		rl = resp.RateLimits
	}
	if rl == nil {
		return nil, errors.New("no rateLimits object in Codex response")
	}

	var models []ModelQuota
	if rl.Primary != nil {
		rem := 100 - rl.Primary.UsedPercent
		if rem < 0 {
			rem = 0
		}
		refreshes := ""
		if rl.Primary.ResetsAt > 0 {
			refreshes = formatDate(time.Unix(rl.Primary.ResetsAt, 0).Local())
		}
		window := "5-Hour"
		if rl.Primary.WindowDurationMins > 0 && rl.Primary.WindowDurationMins != 300 {
			window = fmt.Sprintf("%dm", rl.Primary.WindowDurationMins)
		}
		models = append(models, ModelQuota{
			Model:     fmt.Sprintf("Primary (%s)", window),
			Percent:   fmt.Sprintf("%d%% remaining", rem),
			Refreshes: refreshes,
		})
	}
	if rl.Secondary != nil {
		rem := 100 - rl.Secondary.UsedPercent
		if rem < 0 {
			rem = 0
		}
		refreshes := ""
		if rl.Secondary.ResetsAt > 0 {
			refreshes = formatDate(time.Unix(rl.Secondary.ResetsAt, 0).Local())
		}
		window := "Weekly"
		if rl.Secondary.WindowDurationMins > 0 && rl.Secondary.WindowDurationMins != 10080 {
			window = fmt.Sprintf("%dm", rl.Secondary.WindowDurationMins)
		}
		models = append(models, ModelQuota{
			Model:     fmt.Sprintf("Secondary (%s)", window),
			Percent:   fmt.Sprintf("%d%% remaining", rem),
			Refreshes: refreshes,
		})
	}

	if len(models) == 0 {
		return nil, errors.New("no rate limit windows found in Codex response")
	}
	return models, nil
}

// Format formats agent quotas into the target output specification:
//
//	[Agent]
//	    [Model] [Percent] (Refreshes <Date>)
func Format(quotas []AgentQuota) string {
	return FormatColored(quotas, false)
}

var pctRE = regexp.MustCompile(`(\d+)%`)

// PercentColor returns ANSI color code based on percentage value:
// Green ("32") if above 80%, yellow ("33") if above 40%, otherwise red ("31").
func PercentColor(val int) string {
	if val > 80 {
		return "32" // Green
	}
	if val > 40 {
		return "33" // Yellow
	}
	return "31" // Red
}

// ColorizePercent wraps the percentage text in ANSI color according to its value.
func ColorizePercent(text string, enabled bool) string {
	if !enabled {
		return text
	}
	m := pctRE.FindStringSubmatch(text)
	if m == nil {
		return text
	}
	val, err := strconv.Atoi(m[1])
	if err != nil {
		return text
	}
	return "\x1b[" + PercentColor(val) + "m" + text + "\x1b[0m"
}

// FormatColored formats agent quotas, optionally colorizing the percentage text.
func FormatColored(quotas []AgentQuota, colorize bool) string {
	var b strings.Builder
	for _, a := range quotas {
		b.WriteString(a.Agent + "\n")
		if a.Error != nil {
			b.WriteString(fmt.Sprintf("    Unavailable: %v\n", a.Error))
			continue
		}
		for _, m := range a.Models {
			pct := ColorizePercent(m.Percent, colorize)
			b.WriteString(fmt.Sprintf("    %s %s (Refreshes %s)\n", m.Model, pct, m.Refreshes))
		}
	}
	return b.String()
}

// Service queries and formats quotas for all agents.
type Service struct {
	RunClaude func(ctx context.Context) (string, error)
	RunAgy    func(ctx context.Context) (string, error)
	RunCodex  func(ctx context.Context) (string, error)
	Now       func() time.Time
}

func resolveCmd(name string) string {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath(name + ".cmd"); err == nil {
			return p
		}
		if p, err := exec.LookPath(name + ".exe"); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

// DefaultRunClaude executes `claude --print "/usage"`.
func DefaultRunClaude(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, resolveCmd("claude"), "--print", "/usage")
	cmd.Stdin = strings.NewReader("\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude usage failed (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// DefaultRunAgy executes `agy --print "/usage"`.
func DefaultRunAgy(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, resolveCmd("agy"), "--print", "/usage")
	cmd.Stdin = strings.NewReader("\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("agy usage failed (%w): %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// DefaultRunCodex communicates with `codex app-server` via JSON-RPC.
func DefaultRunCodex(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, resolveCmd("codex"), "app-server")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"work_dot_bat","version":"1.0"}}}` + "\n"
	if _, err := io.WriteString(stdin, initReq); err != nil {
		return "", err
	}
	rlReq := `{"jsonrpc":"2.0","id":2,"method":"account/rateLimits/read","params":{}}` + "\n"
	if _, err := io.WriteString(stdin, rlReq); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"id":2`) || strings.Contains(line, `"id": 2`) {
			return line, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("no rate limit response from codex app-server")
}

// NewService returns a Service configured with the default CLI runners.
func NewService() *Service {
	return &Service{
		RunClaude: DefaultRunClaude,
		RunAgy:    DefaultRunAgy,
		RunCodex:  DefaultRunCodex,
		Now:       time.Now,
	}
}

// Fetch queries agent quotas concurrently and returns them.
func (s *Service) Fetch(ctx context.Context, agentFilter string) ([]AgentQuota, error) {
	type task struct {
		name string
		run  func(ctx context.Context) (string, error)
		par  func(string, time.Time) ([]ModelQuota, error)
	}

	tasks := []task{
		{"Claude", s.RunClaude, ParseClaudeUsage},
		{"AntiGravity", s.RunAgy, ParseAgyUsage},
		{"Codex", s.RunCodex, ParseCodexRateLimits},
	}

	matchesAgent := func(agentName, filter string) bool {
		f := strings.ToLower(filter)
		name := strings.ToLower(agentName)
		if strings.Contains(name, f) {
			return true
		}
		if name == "antigravity" && (f == "agy" || f == "gemini") {
			return true
		}
		if name == "claude" && (f == "anthropic" || f == "sonnet" || f == "opus") {
			return true
		}
		if name == "codex" && (f == "openai" || f == "gpt") {
			return true
		}
		return false
	}

	filter := strings.ToLower(strings.TrimSpace(agentFilter))
	if filter != "" {
		var filtered []task
		for _, t := range tasks {
			if matchesAgent(t.name, filter) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("unknown agent %q; expected claude, agy/antigravity, or codex", agentFilter)
		}
		tasks = filtered
	}

	results := make([]AgentQuota, len(tasks))
	var wg sync.WaitGroup
	now := s.Now()

	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, tk task) {
			defer wg.Done()
			results[idx].Agent = tk.name
			raw, err := tk.run(ctx)
			if err != nil {
				results[idx].Error = err
				return
			}
			models, err := tk.par(raw, now)
			if err != nil {
				results[idx].Error = err
				return
			}
			results[idx].Models = models
		}(i, t)
	}

	wg.Wait()
	return results, nil
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// Run executes the quota command with given args and prints the formatted output.
func Run(args []string, stdout, stderr io.Writer) int {
	filter := ""
	colorFlag := false
	noColorFlag := false
	for _, a := range args {
		if strings.HasPrefix(a, "project=") {
			continue
		}
		if a == "--color" {
			colorFlag = true
			continue
		}
		if a == "--no-color" {
			noColorFlag = true
			continue
		}
		if filter == "" && !strings.HasPrefix(a, "-") {
			filter = a
		}
	}

	useColor := isTerminal(stdout) || colorFlag
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		useColor = false
	}

	svc := NewService()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	quotas, err := svc.Fetch(ctx, filter)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Fprint(stdout, FormatColored(quotas, useColor))
	return 0
}
