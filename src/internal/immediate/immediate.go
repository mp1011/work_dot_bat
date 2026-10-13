// Package immediate owns the fixed "immediate" plan, agent_docs/plans/immediate.md: a one-slot
// inbox where failed automations are recorded. The launcher, loop mode and daemon pick it ahead
// of every other plan while it is not Done; the Work script (never the agent) sets it Done and
// erases its body once every recorded automation passes.
package immediate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/planstatus"
)

// Plan is the fixed path, relative to agent_docs/ as plan-status takes it.
const Plan = "plans/immediate.md"

// MaxOutput caps the captured output kept per failure; the tail is what matters.
const MaxOutput = 6000

const sectionPrefix = "## AUTOMATION: "

// Failure is one failed automation.
type Failure struct {
	Name    string // identifies the automation (the check name)
	Command string
	Output  string
}

// FilePath is where the plan lives on disk under root.
func FilePath(root string) string {
	return filepath.Join(root, "agent_docs", filepath.FromSlash(Plan))
}

// Pending reports whether the plan exists and is not Done.
func Pending(root string) bool {
	status, ok := planstatus.ReadEntryStatus(root, Plan)
	return ok && status != "Done"
}

// Record writes the failures into the plan, creating it when absent. A failure of an automation
// already recorded replaces that automation's section; others are appended. After Done, the
// file starts over with a fresh body.
func Record(root string, failures []Failure) error {
	if len(failures) == 0 {
		return nil
	}
	status, tracked := planstatus.ReadEntryStatus(root, Plan)
	var sections []section
	if tracked && status != "Done" {
		body, _ := planstatus.ReadBody(root, Plan)
		sections = parse(body)
	}
	for _, f := range failures {
		sections = upsert(sections, section{name: f.Name, text: render(f)})
	}
	body := renderBody(sections)

	if !tracked {
		if err := os.MkdirAll(filepath.Dir(FilePath(root)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(FilePath(root), []byte(body), 0o644); err != nil {
			return err
		}
		return planstatus.Add(root, Plan, "Not Started")
	}
	if err := planstatus.SetBody(root, Plan, body); err != nil {
		return err
	}
	if status != "In Progress" {
		return planstatus.Set(root, Plan, "Not Started", false)
	}
	return nil
}

// Clear removes the sections of automations that passed. When none remain, the plan is set Done
// through plan-status and its body is erased. A pass of an unrecorded automation changes nothing.
func Clear(root string, passed []string) error {
	status, ok := planstatus.ReadEntryStatus(root, Plan)
	if !ok || status == "Done" {
		return nil
	}
	body, _ := planstatus.ReadBody(root, Plan)
	sections := parse(body)
	kept := sections[:0:0]
	for _, s := range sections {
		if !contains(passed, s.name) {
			kept = append(kept, s)
		}
	}
	if len(kept) == len(sections) && len(sections) > 0 {
		return nil
	}
	if len(kept) > 0 {
		return planstatus.SetBody(root, Plan, renderBody(kept))
	}
	if err := planstatus.Set(root, Plan, "Done", false); err != nil {
		return err
	}
	return planstatus.SetBody(root, Plan, "")
}

// Recorded returns the names of the automations currently recorded in the plan.
func Recorded(root string) []string {
	status, ok := planstatus.ReadEntryStatus(root, Plan)
	if !ok || status == "Done" {
		return nil
	}
	body, _ := planstatus.ReadBody(root, Plan)
	var names []string
	for _, s := range parse(body) {
		names = append(names, s.name)
	}
	return names
}

type section struct{ name, text string }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func upsert(sections []section, s section) []section {
	for i := range sections {
		if sections[i].name == s.name {
			sections[i] = s
			return sections
		}
	}
	return append(sections, s)
}

// parse splits a body into its automation sections; text before the first one is dropped (it is
// regenerated).
func parse(body string) []section {
	var out []section
	var cur *section
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, sectionPrefix) {
			out = append(out, section{name: strings.TrimSpace(strings.TrimPrefix(line, sectionPrefix))})
			cur = &out[len(out)-1]
		}
		if cur != nil {
			cur.text += line + "\n"
		}
	}
	for i := range out {
		out[i].text = strings.TrimRight(out[i].text, "\n") + "\n"
	}
	return out
}

func render(f Failure) string {
	out := tail(strings.TrimRight(f.Output, "\r\n"), MaxOutput)
	// A fence longer than any backtick run in the output keeps the section parseable.
	fence := "```"
	for strings.Contains(out, fence) {
		fence += "`"
	}
	return fmt.Sprintf("%s%s\nCommand: `%s`\n\n%s\n%s\n%s\n", sectionPrefix, oneLine(f.Name), oneLine(f.Command), fence, out, fence)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// tail keeps the last max bytes, on a line boundary where one is near.
func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[len(s)-max:]
	if i := strings.IndexByte(cut, '\n'); i >= 0 && i < len(cut)/2 {
		cut = cut[i+1:]
	}
	return "[… output truncated; showing the end]\n" + cut
}

func renderBody(sections []section) string {
	var b strings.Builder
	b.WriteString("# Immediate: fix the failing automation(s)\n\n")
	b.WriteString("## Goal\nMake every automation below pass. The Work script marks this plan Done and clears it once they all do; do not change its status or edit this file.\n\n")
	for _, s := range sections {
		b.WriteString(s.text)
		b.WriteString("\n")
	}
	return b.String()
}

// Due says whether the plan should be picked ahead of everything else.
func Due(root string) (entry planstatus.Entry, due bool) {
	status, ok := planstatus.ReadEntryStatus(root, Plan)
	if !ok || status == "Done" {
		return planstatus.Entry{}, false
	}
	return planstatus.Entry{Path: Plan, Status: status}, true
}

// Notice is the line printed when the plan is picked.
func Notice() string {
	return "Immediate plan pending: picking agent_docs/" + Plan
}
