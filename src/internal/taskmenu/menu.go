package taskmenu

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"work_dot_bat/src/internal/menu"
	"work_dot_bat/src/internal/planstatus"
)

// Menu is the task picker. Tasks, EnsureFrontmatter and AddPlan are seams for tests; New
// wires them to the real plan-status code.
type Menu struct {
	menu.IO
	Root  string
	IsTTY bool
	// Tasks is every plan overlaid with worktree status (done and parked ones included).
	Tasks func() []planstatus.Entry
	// EnsureFrontmatter adds "Not Started" frontmatter to plan files lacking it; it returns
	// the agent_docs-relative paths it touched.
	EnsureFrontmatter func() []string
	// AddPlan registers a new plan file as "Not Started".
	AddPlan func(entryPath string) error
}

func New(io menu.IO, root string, isTTY bool) Menu {
	cli := planstatus.CLI{Root: root, Collect: planstatus.GitCollectWorktreeStatuses}
	return Menu{
		IO: io, Root: root, IsTTY: isTTY,
		Tasks:             cli.OverlaidEntries,
		EnsureFrontmatter: func() []string { return planstatus.EnsureFrontmatter(root) },
		AddPlan:           func(p string) error { return planstatus.Add(root, p, "Not Started") },
	}
}

// Result is what the menu hands back: a chosen plan, none (skip, or input ended), or Quit.
type Result struct {
	Task *Item
	Quit bool
}

var leadingIntRE = regexp.MustCompile(`^\s*[+-]?\d+`)

// parseInt mirrors JavaScript's parseInt(s, 10): the leading integer, ok=false if none.
func parseInt(s string) (int, bool) {
	m := leadingIntRE.FindString(s)
	if m == "" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(m))
	return n, err == nil
}

func (m Menu) ask(prompt string) (string, bool) {
	line, ok := m.Ask(prompt)
	return strings.TrimSpace(line), ok
}

func isQuit(lower string) bool { return lower == "q" || lower == "quit" }

func (m Menu) quit() Result {
	m.Log("Exiting work script.")
	return Result{Quit: true}
}

func (m Menu) newItem(p string) *Item {
	return &Item{Entry: planstatus.Entry{Path: p, Status: "Not Started"}}
}

// Select runs the "Step 1" menu to a choice.
func (m Menu) Select() Result {
	for _, p := range m.EnsureFrontmatter() {
		m.Log(fmt.Sprintf(`Added "Not Started" frontmatter to agent_docs/%s`, p))
	}
	tasks := m.Tasks()
	listed := RunnableAndGrouped(tasks, m.Root)
	parked := ParkedSuggestions(tasks)

	viewLine, viewHint := "", ""
	if n := len(parked); n > 0 {
		plural := "s"
		if n == 1 {
			plural = ""
		}
		viewLine = fmt.Sprintf("  v) [View %d parked suggestion%s]", n, plural)
		viewHint = fmt.Sprintf(", 'v' to view %d parked suggestion(s)", n)
	}

	printList := func() {
		if len(listed) > 0 {
			m.Log("Incomplete Tasks:\n")
			for i, t := range listed {
				m.Log(FormatTaskLine(t, i, listed, m.IsTTY))
			}
			m.Log("  w) [Write-in task]")
			m.Log("  p) [Paste path to a new plan file]")
			if viewLine != "" {
				m.Log(viewLine)
			}
			m.Log("  0) [Skip / No task selected]")
			m.Log("")
			return
		}
		m.Log("No incomplete tasks found in Task Index.\n")
		m.Log("  w) [Write-in task]")
		m.Log("  p) [Paste path to a new plan file]")
		if viewLine != "" {
			m.Log(viewLine)
		}
		m.Log("  0) [Skip / No task selected]\n")
	}
	prompt := fmt.Sprintf("Select a task (1-%d), 'w' for write-in task, 'p' to paste path%s, '0' / 's' to skip, or 'q' to quit: ", len(listed), viewHint)
	invalid := "Invalid selection \"%s\". Please enter a number between 1 and %d, 'w' for write-in task, 'p' to paste a plan path, '0' to skip, or 'q' to quit."
	if len(listed) == 0 {
		prompt = fmt.Sprintf("Select option ('w' for write-in task, 'p' to paste path%s, '0' / 's' to skip, or 'q' to quit): ", viewHint)
		invalid = "Invalid selection \"%[1]s\". Please enter 'w' for write-in task, 'p' to paste a plan path, '0' to skip, or 'q' to quit."
	}

	printList()
	for {
		input, ok := m.ask(prompt)
		if !ok {
			return Result{}
		}
		lower := strings.ToLower(input)
		switch {
		case isQuit(lower):
			return m.quit()
		case lower == "v" || lower == "view":
			sel, quit, ok := m.selectSuggestion(parked)
			if quit {
				return m.quit()
			}
			if !ok {
				return Result{}
			}
			if sel != nil {
				return Result{Task: &Item{Entry: *sel}}
			}
			m.Log("")
			printList()
		case lower == "0" || lower == "s" || lower == "skip" || lower == "":
			return Result{}
		case lower == "w" || lower == "write" || lower == "write-in":
			item, ok := m.writeIn()
			if !ok {
				return Result{}
			}
			if item != nil {
				return Result{Task: item}
			}
		case lower == "p" || lower == "paste" || lower == "add" || lower == "new":
			raw, ok := m.ask("Enter or paste plan file path: ")
			if !ok {
				return Result{}
			}
			if item := m.pastePath(raw); item != nil {
				return Result{Task: item}
			}
		default:
			if n, isNum := parseInt(input); isNum && n >= 1 && n <= len(listed) && len(listed) > 0 {
				t := listed[n-1]
				return Result{Task: &t}
			}
			if t := findListed(listed, input); t != nil {
				return Result{Task: t}
			}
			if item := m.pastePath(input); item != nil {
				return Result{Task: item}
			}
			m.Log(fmt.Sprintf(invalid, input, len(listed)))
		}
	}
}

func findListed(listed []Item, input string) *Item {
	clean := strings.TrimSpace(input)
	for _, t := range listed {
		disp := t.DisplayPath
		if disp == "" {
			disp = t.Path
		}
		if t.Path == clean || t.Path == "plans/"+clean ||
			disp == clean || disp == "plans/"+clean {
			t := t
			return &t
		}
	}
	return nil
}

// selectSuggestion lists the parked suggestions and prompts for one. sel is nil for "back";
// ok is false when input ended.
func (m Menu) selectSuggestion(suggestions []planstatus.Entry) (sel *planstatus.Entry, quit, ok bool) {
	if len(suggestions) == 0 {
		m.Log("\nNo parked suggestions.\n")
		return nil, false, true
	}
	m.Log(fmt.Sprintf("\n%d parked suggestion(s) — agent_docs/plans/agent_suggestions/:\n", len(suggestions)))
	for _, line := range FormatSuggestionList(suggestions, m.Root) {
		m.Log(line)
	}
	m.Log("")
	for {
		input, more := m.ask(fmt.Sprintf("Select a suggestion (1-%d), 'b' to go back, or 'q' to quit: ", len(suggestions)))
		if !more {
			return nil, false, false
		}
		lower := strings.ToLower(input)
		switch lower {
		case "q", "quit":
			return nil, true, true
		case "b", "back", "0", "s", "skip":
			return nil, false, true
		}
		if n, isNum := parseInt(input); isNum && n >= 1 && n <= len(suggestions) {
			return &suggestions[n-1], false, true
		}
		for i, s := range suggestions {
			if s.Path == input || s.Path == "plans/"+input || s.Path == "plans/agent_suggestions/"+input ||
				s.Path == "plans/agent_suggestions/"+input+".md" || strings.ToLower(suggestionStem(s.Path)) == lower {
				return &suggestions[i], false, true
			}
		}
		m.Log(fmt.Sprintf("Invalid selection \"%s\". Please enter a number between 1 and %d, 'b' to go back, or 'q' to quit.", input, len(suggestions)))
	}
}

// pastePath registers a plan file the user pointed at, or finds it already tracked.
func (m Menu) pastePath(raw string) *Item {
	entry := ResolvePlanPath(raw, m.Root)
	if entry == "" {
		m.Log(fmt.Sprintf("\n[ERROR] Invalid path. Plan file must be inside agent_docs/: \"%s\"", raw))
		return nil
	}
	if !fileExists(filepath.Join(m.Root, "agent_docs", filepath.FromSlash(entry))) {
		m.Log(fmt.Sprintf("\n[ERROR] Plan file does not exist: agent_docs/%s", entry))
		return nil
	}
	for _, t := range m.Tasks() {
		if t.Path == entry {
			m.Log(fmt.Sprintf("\nPlan \"%s\" is already in the plan index.", entry))
			return &Item{Entry: t}
		}
	}
	return m.register(entry, fmt.Sprintf("\nInserting \"%s\" into plan index...", entry))
}

func (m Menu) register(entry, intro string) *Item {
	m.Log(intro)
	if err := m.AddPlan(entry); err != nil {
		m.Log(err.Error())
		m.Log(fmt.Sprintf("\n[ERROR] Failed to insert \"%s\" into plan index.", entry))
		return nil
	}
	m.Log(fmt.Sprintf("Successfully added \"%s\" to plan index!", entry))
	return m.newItem(entry)
}

func (m Menu) promptFolder() (string, bool) {
	folders := PlanFolders(m.Root)
	m.Log("\nSelect target folder for new plan file:\n")
	for i, f := range folders {
		m.Log(fmt.Sprintf("  %d) %s", i+1, f))
	}
	m.Log("")
	for {
		answer, ok := m.ask(fmt.Sprintf("Choose folder (1-%d): ", len(folders)))
		if !ok {
			return "", false
		}
		if n, isNum := parseInt(answer); isNum && n >= 1 && n <= len(folders) {
			return folders[n-1], true
		}
		cleaned := strings.Trim(toSlash(answer), "/")
		for _, f := range folders {
			if strings.EqualFold(f, cleaned) || strings.EqualFold(f, "plans/"+cleaned) {
				return f, true
			}
		}
		m.Log(fmt.Sprintf("Invalid selection \"%s\". Please select a number between 1 and %d.", answer, len(folders)))
	}
}

func (m Menu) promptFilename() (string, bool) {
	for {
		raw, ok := m.ask("\nEnter filename (which may include a new folder): ")
		if !ok {
			return "", false
		}
		cleaned := toSlash(stripQuotes(raw))
		if cleaned == "" {
			m.Log("Filename cannot be empty. Please enter a valid filename.")
			continue
		}
		if !strings.HasSuffix(strings.ToLower(cleaned), ".md") {
			cleaned += ".md"
		}
		return cleaned, true
	}
}

// writeIn creates a new plan file from a folder, filename and problem description. A nil
// item with ok means it failed and the menu should prompt again; ok=false is end of input.
func (m Menu) writeIn() (item *Item, ok bool) {
	folder, ok := m.promptFolder()
	if !ok {
		return nil, false
	}
	filename, ok := m.promptFilename()
	if !ok {
		return nil, false
	}
	combined := path.Join(folder, filename)
	entry := ResolvePlanPath(combined, m.Root)
	if entry == "" {
		m.Log(fmt.Sprintf("\n[ERROR] Invalid path. Plan file must be inside agent_docs/: \"%s\"", combined))
		return nil, true
	}
	desc, ok := m.ask("\nBriefly describe the problem / goal for this task: ")
	if !ok {
		return nil, false
	}
	full := filepath.Join(m.Root, "agent_docs", filepath.FromSlash(entry))
	if fileExists(full) {
		m.Log(fmt.Sprintf("\n[ERROR] Plan file already exists: agent_docs/%s", entry))
		return nil, true
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		m.Log(fmt.Sprintf("\n[ERROR] %v", err))
		return nil, true
	}
	if err := os.WriteFile(full, []byte("# GOAL\n"+desc+"\n"), 0o644); err != nil {
		m.Log(fmt.Sprintf("\n[ERROR] %v", err))
		return nil, true
	}
	m.Log(fmt.Sprintf("\nCreated plan file at: agent_docs/%s", entry))
	return m.register(entry, fmt.Sprintf("Inserting \"%s\" into plan index...", entry)), true
}
