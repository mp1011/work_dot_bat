// Package menu is the project-selection menu and the "paste a path" registration flow.
// Input goes through Ask and output through Log, so the flow is testable.
package menu

import (
	"fmt"
	"path/filepath"
	"strings"

	"work_dot_bat/internal/projectconfig"
	"work_dot_bat/internal/projects"
	"work_dot_bat/internal/wrappers"
)

type IO struct {
	// Ask prints a prompt and returns the next input line; ok is false at end of input.
	Ask func(prompt string) (line string, ok bool)
	Log func(msg string)
}

type Options struct {
	IO
	Projects     []projects.Project
	RegistryFile string
	// ToolRoot, when set, is where the bin launchers live; a newly registered repo gets .tools
	// wrappers pointing there.
	ToolRoot string
	// Bootstrap, when non-nil, is offered after registering a repo (ported in a later subtask).
	Bootstrap func(p projects.Project) (string, error)
}

func promptCommands(io IO) (projectconfig.Config, bool) {
	defaults := projectconfig.NpmDefaults()
	defaultFor := map[string]*projectconfig.Command{
		"lightTest": defaults.LightTest, "heavyTest": defaults.HeavyTest,
		"install": defaults.Install, "devServer": defaults.DevServer,
	}
	var config projectconfig.Config
	for _, field := range projectconfig.CommandFields {
		var shown string
		if field == "build" {
			parts := make([]string, len(defaults.Build))
			for i, c := range defaults.Build {
				parts[i] = projectconfig.FormatCommand(c)
			}
			shown = strings.Join(parts, " && ")
		} else {
			shown = projectconfig.FormatCommand(*defaultFor[field])
		}
		line, ok := io.Ask(fmt.Sprintf(`  %s [%s] (Enter = default, "none" = skip): `, field, shown))
		if !ok {
			return config, false
		}
		answer := strings.TrimSpace(line)
		switch {
		case answer == "":
			if field == "build" {
				config.Build = defaults.Build
			} else {
				set(&config, field, defaultFor[field])
			}
		case strings.EqualFold(answer, "none"):
		case field == "build":
			// `build` is a list; the prompt takes `&&`-separated commands.
			for _, part := range strings.Split(answer, "&&") {
				if c := projectconfig.ParseCommandLine(part); c != nil {
					config.Build = append(config.Build, *c)
				}
			}
		default:
			set(&config, field, projectconfig.ParseCommandLine(answer))
		}
	}
	return config, true
}

func set(c *projectconfig.Config, field string, v *projectconfig.Command) {
	switch field {
	case "lightTest":
		c.LightTest = v
	case "heavyTest":
		c.HeavyTest = v
	case "install":
		c.Install = v
	case "devServer":
		c.DevServer = v
	}
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// RegisterPastedPath returns the registered project, or nil if the user gave up (or input
// ended). Re-prompts on bad input.
func RegisterPastedPath(o Options) *projects.Project {
	for {
		line, ok := o.Ask("Repository path (blank to cancel): ")
		raw := unquote(strings.TrimSpace(line))
		if !ok || raw == "" {
			return nil
		}
		if good, reason := projects.CheckRepoRoot(raw); !good {
			o.Log("  " + reason)
			continue
		}
		suggested := projects.DefaultProjectName(raw)
		var name string
		for {
			line, ok := o.Ask(fmt.Sprintf("Project name [%s]: ", suggested))
			if !ok {
				return nil
			}
			name = strings.TrimSpace(line)
			if name == "" {
				name = suggested
			}
			if projects.IsValidProjectName(name) {
				break
			}
			o.Log(`  Names must be non-empty and contain no spaces or "=".`)
		}
		if _, err := projects.AddProject(o.Projects, projects.Project{Name: name, Path: raw}); err != nil {
			o.Log("  " + err.Error())
			continue
		}
		newProj := projects.Project{Name: name, Path: raw}
		existing := projectconfig.Load(raw)
		if existing.Exists {
			o.Log(fmt.Sprintf("  Using the existing %s", filepath.Join(raw, projectconfig.ConfigFile)))
			for _, e := range existing.Errors {
				o.Log("  WARNING: " + e)
			}
		} else {
			o.Log("No work_dot_bat.config.json in that repo yet. Commands for this project:")
			commands, ok := promptCommands(o.IO)
			if !ok {
				return nil
			}
			newProj.Build = commands.Build
			newProj.Install = commands.Install
			newProj.DevServer = commands.DevServer
			newProj.ProjectStructureDoc = "agent_docs/02_project_structure.md"
			newProj.ProjectStructureDirs = []string{"src"}
			if commands.LightTest != nil {
				newProj.Tests.Light = [][]string{append([]string{commands.LightTest.Cmd}, commands.LightTest.Args...)}
			}
			if commands.HeavyTest != nil {
				newProj.Tests.Heavy = [][]string{append([]string{commands.HeavyTest.Cmd}, commands.HeavyTest.Args...)}
			}
		}
		updated, _ := projects.AddProject(o.Projects, newProj)
		if err := projects.SaveRegistry(o.RegistryFile, updated); err != nil {
			o.Log("  " + err.Error())
			return nil
		}
		o.Log(fmt.Sprintf("Registered %q.", name))
		if o.ToolRoot != "" {
			if _, err := wrappers.Install(raw, o.ToolRoot, name); err != nil {
				o.Log("  Could not write " + wrappers.Dir + " wrappers: " + err.Error())
			} else {
				o.Log(fmt.Sprintf("  Wrote %s wrappers (git-ignored): run %s/work from the repo.", wrappers.Dir, wrappers.Dir))
			}
		}
		project := projects.FindProject(updated, name)
		// Offered, never forced: the user can run `bootstrap` later.
		if o.Bootstrap != nil {
			line, _ := o.Ask("Bootstrap this repo now (agent docs, skills, vendored scripts, hooks)? (y/N) ")
			if a := strings.ToLower(strings.TrimSpace(line)); a == "y" || a == "yes" {
				if report, err := o.Bootstrap(*project); err != nil {
					o.Log("  Bootstrap failed: " + err.Error())
				} else {
					o.Log(report)
				}
			}
		}
		return project
	}
}

// Run shows the menu until the user picks a project, registers one, or quits (nil).
// End of input counts as quit.
func Run(o Options) *projects.Project {
	for {
		o.Log(projects.FormatMenu(o.Projects))
		line, ok := o.Ask("> ")
		if !ok {
			return nil
		}
		choice := projects.ParseMenuChoice(line, o.Projects)
		switch choice.Kind {
		case projects.ChoiceProject:
			p := choice.Project
			return &p
		case projects.ChoiceQuit:
			return nil
		case projects.ChoicePaste:
			if p := RegisterPastedPath(o); p != nil {
				return p
			}
		default:
			o.Log("Invalid choice.")
		}
	}
}
