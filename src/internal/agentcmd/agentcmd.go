// Package agentcmd resolves agent CLI settings (tool defaults, the optional agent_settings.json,
// the project's `agents` block) and builds agent command lines and prompts. All pure except
// the settings-file read and the one agent-choice prompt.
package agentcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/projects"
	"work_dot_bat/src/internal/repo"
)

const SettingsEnv = "WORK_DOT_BAT_AGENT_SETTINGS"

// AgentSettings is one agent's settings; keys are cmd/model/permissionMode/effort, but
// overrides may add others, as in the Node object-assign merge.
type AgentSettings map[string]any

type Settings struct {
	Claude, Agy AgentSettings
}

func (a AgentSettings) str(key string) string {
	switch v := a[key].(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func DefaultSettings() Settings {
	return Settings{
		Claude: AgentSettings{"cmd": "claude", "model": "sonnet", "permissionMode": "auto"},
		Agy:    AgentSettings{"cmd": "agy", "model": "gemini-3.8-flash", "effort": "medium"},
	}
}

func DefaultSettingsFile() string {
	if v := os.Getenv(SettingsEnv); v != "" {
		return v
	}
	return filepath.Join(projects.ToolRoot(), "agent_settings.json")
}

func merge(base Settings, override map[string]any) Settings {
	out := Settings{Claude: AgentSettings{}, Agy: AgentSettings{}}
	for k, v := range base.Claude {
		out.Claude[k] = v
	}
	for k, v := range base.Agy {
		out.Agy[k] = v
	}
	for name, dst := range map[string]AgentSettings{"claude": out.Claude, "agy": out.Agy} {
		if o, ok := override[name].(map[string]any); ok {
			for k, v := range o {
				dst[k] = v
			}
		}
	}
	return out
}

// ResolveSettings layers defaults, the settings file, then projectAgents (the project config's
// `agents` block, may be nil). An unreadable or invalid file is ignored, not a failure.
func ResolveSettings(file string, projectAgents map[string]any) Settings {
	s := DefaultSettings()
	if b, err := os.ReadFile(file); err == nil {
		var parsed any
		if json.Unmarshal(b, &parsed) == nil {
			if m, ok := parsed.(map[string]any); ok {
				s = merge(s, m)
			}
		}
	}
	return merge(s, projectAgents)
}

// ProjectSettings applies the selected project's `agents` overrides. Never fails: a project
// without a (valid) config gets the tool-level settings.
func ProjectSettings(treeRoot, projectRoot string) Settings {
	var agents map[string]any
	if cfg, err := projectconfig.ForProject(treeRoot, projectRoot); err == nil {
		agents, _ = cfg["agents"].(map[string]any)
	}
	return ResolveSettings(DefaultSettingsFile(), agents)
}

// Command is a program and its argv.
type Command struct {
	Cmd  string
	Args []string
}

func IsAgy(agent string) bool { return agent == "a" || agent == "agy" }

type CommandOptions struct {
	RepoRoot    string
	Prompt      string
	Interactive bool
	SessionID   string // claude only
	LogFile     string // agy only
}

// BuildCommand builds the command line. agent is the ledger vocabulary: "a"/"agy" for
// Antigravity, anything else Claude. Interactive keeps the terminal; otherwise one-shot.
func BuildCommand(agent string, o CommandOptions, s Settings) Command {
	if IsAgy(agent) {
		a := s.Agy
		args := []string{"--add-dir", o.RepoRoot, "--model", a.str("model"), "--effort", a.str("effort")}
		if o.LogFile != "" {
			args = append(args, "--log-file", o.LogFile)
		}
		if o.Interactive {
			args = append(args, "-i", o.Prompt)
		} else {
			args = append(args, "--print", o.Prompt)
		}
		return Command{a.str("cmd"), args}
	}
	c := s.Claude
	args := []string{"--model", c.str("model"), "--permission-mode", c.str("permissionMode")}
	if o.SessionID != "" {
		args = append(args, "--session-id", o.SessionID)
	}
	if !o.Interactive {
		args = append(args, "--print")
	}
	args = append(args, o.Prompt)
	return Command{c.str("cmd"), args}
}

type InvocationOptions struct {
	SessionName string
	SessionID   string
	LogFile     string
}

// BuildInvocation is BuildCommand non-interactive, defaulting the agy log file from the
// session name.
func BuildInvocation(agent, repoRoot, prompt string, o InvocationOptions, s Settings) Command {
	log := o.LogFile
	if log == "" && o.SessionName != "" {
		log = repo.StateDirName + "/logs/" + o.SessionName + ".log"
	}
	return BuildCommand(agent, CommandOptions{RepoRoot: repoRoot, Prompt: prompt, SessionID: o.SessionID, LogFile: log}, s)
}

// BuildDoWorkYoloPrompt: launcherHoldsClaim tells the agent the launcher owns the claim's
// acquire and release (and the Ready for Review transition).
func BuildDoWorkYoloPrompt(planPath string, launcherHoldsClaim bool) string {
	base := "/do-work-yolo " + planPath
	if !launcherHoldsClaim {
		return base
	}
	return base + "\n\nNote: the launcher that spawned you already holds the plan/branch claim and " +
		"releases it when you exit. Skip STEP ZERO's acquire and do not run `plan-claim.sh release`; " +
		"still run the `concurrent-check` read. " +
		"Do not mark the plan 'Ready for Review' — the automated test harness runs verification after you " +
		"exit and sets Ready for Review only after all checks pass."
}

func BuildFailureFollowupPrompt(planPath, planContent string, failures []string, attempt, maxAttempts int) string {
	lines := []string{
		fmt.Sprintf("Automated tests failed after working on the plan at %s (attempt %d of %d).", planPath, attempt, maxAttempts),
		"",
		"Failing checks:",
	}
	for _, f := range failures {
		lines = append(lines, "- "+f)
	}
	lines = append(lines,
		"",
		fmt.Sprintf("Here is the plan file (%s) for context:", planPath),
		"",
		planContent,
		"",
		"Investigate the failure(s) — re-run the specific failing check(s) yourself to see the full error "+
			"output — and fix them. Do not commit or push.",
	)
	return strings.Join(lines, "\n")
}

// LauncherClaimNote is appended to a prompt when the interactive launcher holds the plan/branch
// claim for the agent's run.
const LauncherClaimNote = "\n\nNote: work.bat already holds the plan/branch claim and releases it when you exit. " +
	"Skip STEP ZERO's acquire and do not run `plan-claim.sh release`; " +
	"still run the `concurrent-check` read."

func BuildRetryPrompt(planPath, currentStatus string, launcherHoldsClaim bool) string {
	claimNote := ""
	if launcherHoldsClaim {
		claimNote = LauncherClaimNote
	}
	return fmt.Sprintf("The previous execution of /do-work-yolo %s ended with status \"%s\" instead of \"Ready for Review\".\n\n", planPath, currentStatus) +
		fmt.Sprintf("Please continue working on this task using /do-work-yolo %s. Complete all remaining requirements, ", planPath) +
		"ensure automated tests pass, and set the task status to \"Ready for Review\" via " +
		fmt.Sprintf("`scripts/work_dot_bat/plan-status.sh set %s \"Ready for Review\"`.", planPath) + claimNote
}

// BuildGeneralAgentPrompt reads "<planPath> for context"; when repoRoot is given and the plan
// file is missing, warn is called with the same notice Node prints.
func BuildGeneralAgentPrompt(planPath, specialInstructions, repoRoot string, warn func(string)) string {
	if repoRoot != "" && warn != nil {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(planPath))); err != nil {
			warn(fmt.Sprintf("\n[WARN] Could not find plan file at %s", planPath))
		}
	}
	prompt := "Read the plan file at " + planPath + " for context."
	if specialInstructions != "" {
		prompt += "\n\n" + specialInstructions
	}
	return prompt
}

// ChooseAgent returns the already-selected agent, else prompts: "a" (Antigravity) or "c"
// (Claude). quit is true on q; ok is false when input ended.
func ChooseAgent(already string, io menu.IO) (agent string, quit, ok bool) {
	if already != "" {
		return already, false, true
	}
	io.Log("\nSelect Agent:")
	io.Log("  (a) Google Antigravity (Gemini 3.8 Flash)")
	io.Log("  (c) Claude Code (Sonnet)\n")
	for {
		line, more := io.Ask("Choose agent (a/c): ")
		if !more {
			return "", false, false
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "a", "antigravity", "1":
			return "a", false, true
		case "c", "claude", "2":
			return "c", false, true
		case "q", "quit":
			io.Log("Exiting work script.")
			return "", true, true
		}
		io.Log(`Invalid agent selection. Please enter "a" for Antigravity or "c" for Claude.`)
	}
}
