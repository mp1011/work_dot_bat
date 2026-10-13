// Package agentcmd resolves the agent definitions (the built-ins plus the project's
// agent_cli.json) and builds agent command lines and prompts. All pure except the
// definitions-file read and the one agent-choice prompt.
package agentcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/repo"
)

// Settings is the set of agents that can be launched.
type Settings struct {
	Defs []Definition // empty = the built-ins
}

func DefaultSettings() Settings { return Settings{Defs: DefaultDefinitions()} }

// ProjectSettings is the built-ins with the project's agent_cli.json applied. Never fails: a
// project without a (valid) file gets the built-ins.
func ProjectSettings(treeRoot, projectRoot string) Settings {
	return Settings{Defs: LoadDefinitions(projectRoot)}
}

// Command is a program and its argv.
type Command struct {
	Cmd  string
	Args []string
}

type CommandOptions struct {
	RepoRoot    string
	Prompt      string
	Interactive bool
	SessionID   string // used when the agent defines sessionIdArgs
	LogFile     string // used when the agent defines logFileArgs
}

// BuildCommand builds the command line for agent (an id, menu key, or alias; unknown names mean
// the first Claude agent) from its definition. Interactive keeps the terminal; otherwise one-shot.
func BuildCommand(agent string, o CommandOptions, s Settings) Command {
	d := s.Agent(agent)
	vars := map[string]string{"prompt": o.Prompt, "repoRoot": o.RepoRoot, "sessionId": o.SessionID, "logFile": o.LogFile}
	args := expand(d.Args, vars)
	if o.SessionID != "" {
		args = append(args, expand(d.SessionIDArgs, vars)...)
	}
	if o.LogFile != "" {
		args = append(args, expand(d.LogFileArgs, vars)...)
	}
	if o.Interactive {
		args = append(args, expand(d.InteractiveArgs, vars)...)
	} else {
		args = append(args, expand(d.PrintArgs, vars)...)
	}
	return Command{d.Cmd, args}
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

// BuildDoWorkYoloPrompt: launcherHoldsClaim tells the agent not to mark 'Ready for Review'
// directly because the automated test harness runs verification after exit.
func BuildDoWorkYoloPrompt(planPath string, launcherHoldsClaim bool) string {
	base := "/do-work-yolo " + planPath
	if !launcherHoldsClaim {
		return base
	}
	return base + "\n\nNote: do not mark the plan 'Ready for Review' — the automated test harness runs verification after you " +
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

// ImmediatePlanNote is appended to the prompt for the immediate plan (agent_docs/plans/immediate.md):
// the Work script owns its status and body, so the agent only fixes the recorded failures.
const ImmediatePlanNote = "\n\nNote: this is the immediate plan. Fix the failures it records. " +
	"Do not run `plan-status.sh` on it (skip the \"Ready for Review\" step) and do not edit or erase the file: " +
	"the Work script sets it Done and clears it when the automations pass."

func BuildRetryPrompt(planPath, currentStatus string, launcherHoldsClaim bool) string {
	return fmt.Sprintf("The previous execution of /do-work-yolo %s ended with status \"%s\" instead of \"Ready for Review\".\n\n", planPath, currentStatus) +
		fmt.Sprintf("Please continue working on this task using /do-work-yolo %s. Complete all remaining requirements, ", planPath) +
		"ensure automated tests pass, and set the task status to \"Ready for Review\" via " +
		fmt.Sprintf("`scripts/work_dot_bat/plan-status.sh set %s \"Ready for Review\"`.", planPath)
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

// ChooseAgent returns the already-selected agent, else prompts with one entry per definition
// (by menu key, id, alias, or position). quit is true on q; ok is false when input ended.
func ChooseAgent(already string, io menu.IO, defs []Definition) (agent string, quit, ok bool) {
	if already != "" {
		return already, false, true
	}
	if len(defs) == 0 {
		defs = DefaultDefinitions()
	}
	io.Log("\nSelect Agent:")
	keys := make([]string, len(defs))
	for i, d := range defs {
		io.Log(fmt.Sprintf("  (%s) %s", d.Key, d.label()))
		keys[i] = d.Key
	}
	io.Log("")
	prompt := "Choose agent (" + strings.Join(keys, "/") + "): "
	for {
		line, more := io.Ask(prompt)
		if !more {
			return "", false, false
		}
		in := strings.ToLower(strings.TrimSpace(line))
		if in == "q" || in == "quit" {
			io.Log("Exiting work script.")
			return "", true, true
		}
		for i, d := range defs {
			if d.matches(in) || in == fmt.Sprint(i+1) {
				return d.Key, false, true
			}
		}
		io.Log("Invalid agent selection. Please enter one of: " + strings.Join(keys, ", ") + ".")
	}
}
