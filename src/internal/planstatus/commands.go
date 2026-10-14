package planstatus

import (
	"fmt"
	"os"
	"path"
	"strings"
)

func errf(format string, a ...any) error { return fmt.Errorf(format, a...) }

func assertValidStatus(status string) error {
	if !contains(Statuses, status) {
		return errf(`Invalid status "%s". Allowed: %s`, status, strings.Join(Statuses, ", "))
	}
	return nil
}

func assertFilterableStatus(status string) error {
	all := append(append([]string{}, Statuses...), DerivedStatuses...)
	if !contains(all, status) {
		return errf(`Invalid status "%s". Allowed: %s`, status, strings.Join(all, ", "))
	}
	return nil
}

func exists(root, rel string) bool {
	_, err := os.Stat(docsPath(root, rel))
	return err == nil
}

func write(root, rel, content string) error {
	return os.WriteFile(docsPath(root, rel), []byte(content), 0o644)
}

// requireUntracked loads a plan file that must exist and must not carry a status yet.
func requireUntracked(root, rel string) (string, error) {
	content, ok := readFile(root, rel)
	if !ok {
		return "", errf("No such file: agent_docs/%s", rel)
	}
	fm := ParseFrontmatter(content)
	if fm.Status != nil {
		return "", errf(`Entry already exists for "%s"`, rel)
	}
	if fm.Informational {
		return "", errf(`"%s" is marked informational — it is not a plan and takes no status`, rel)
	}
	return content, nil
}

// Add writes frontmatter onto a new plan file. A sibling of a plan.md becomes its subtask.
func Add(root, entryPath, status string) error {
	if err := assertValidStatus(status); err != nil {
		return err
	}
	rel := NormalizePlanPath(entryPath)
	content, err := requireUntracked(root, rel)
	if err != nil {
		return err
	}
	expectedParent := path.Dir(rel) + "/plan.md"
	var out string
	if path.Base(rel) != "plan.md" && ParseFrontmatter(content).SubtaskOf == "" && exists(root, expectedParent) {
		out = SetFrontmatterSubtask(content, status, expectedParent)
	} else {
		out = SetFrontmatterStatus(content, status)
	}
	if err := write(root, rel, out); err != nil {
		return err
	}
	WriteShim(root)
	return nil
}

func AddSubtask(root, entryPath, parentPath, status string) error {
	if err := assertValidStatus(status); err != nil {
		return err
	}
	rel, parent := NormalizePlanPath(entryPath), NormalizePlanPath(parentPath)
	if !exists(root, rel) {
		return errf("No such file: agent_docs/%s", rel)
	}
	if !exists(root, parent) {
		return errf("Parent plan file does not exist: agent_docs/%s", parent)
	}
	content, err := requireUntracked(root, rel)
	if err != nil {
		return err
	}
	if err := write(root, rel, SetFrontmatterSubtask(content, status, parent)); err != nil {
		return err
	}
	WriteShim(root)
	return nil
}

func Set(root, entryPath, status string, human bool) error {
	if err := assertValidStatus(status); err != nil {
		return err
	}
	if contains(RestrictedStatuses, status) && !human {
		return errf("Refusing to set restricted status \"%s\".\n"+
			"Cancelled is the human's call (07_plan_index_maintenance.md §2).\n"+
			"If you are the developer, re-run with --human:\n"+
			"  scripts/work_dot_bat/plan-status.sh set %s \"%s\" --human", status, entryPath, status)
	}
	notFound := errf(`No entry found for "%s"`, entryPath)
	content, ok := readFile(root, entryPath)
	if !ok {
		return notFound
	}
	fm := ParseFrontmatter(content)
	if fm.Status == nil {
		return notFound
	}
	if status == "Ready for Review" && fm.Verification == "manual" && !HasManualVerificationSteps(fm.Body) {
		return errf("Refusing to set \"Ready for Review\": \"%s\" is marked verification: manual but has no\n"+
			"\"## Manual Verification\" section with at least one step. Add one (see\n"+
			"agent_docs/page_plan_template.md) before marking it ready for review.", entryPath)
	}
	if status == "Done" {
		norm := NormalizePlanPath(entryPath)
		var open []string
		for _, e := range ReadStatus(root) {
			if NormalizePlanPath(e.SubtaskOf) == norm && e.Status != "Done" && e.Status != "Cancelled" {
				open = append(open, fmt.Sprintf("  ! %s (%s)", e.Path, e.Status))
			}
		}
		if len(open) > 0 {
			return errf("Refusing to set \"%s\" to \"Done\": it has open subtask(s):\n%s\n"+
				"Resolve all subtasks (Done or Cancelled) before marking the parent Done.", entryPath, strings.Join(open, "\n"))
		}
	}
	if err := write(root, entryPath, SetFrontmatterStatus(content, status)); err != nil {
		return err
	}
	WriteShim(root)
	return nil
}

func SetVerification(root, entryPath, verification string) error {
	if verification != "manual" && verification != "automated" {
		return errf(`Invalid verification "%s". Allowed: manual, automated`, verification)
	}
	content, ok := readFile(root, entryPath)
	if !ok || ParseFrontmatter(content).Status == nil {
		return errf(`No entry found for "%s"`, entryPath)
	}
	v := ""
	if verification == "manual" {
		v = "manual"
	}
	if err := write(root, entryPath, SetFrontmatterVerification(content, v)); err != nil {
		return err
	}
	WriteShim(root)
	return nil
}

func SetPlanOrder(root, entryPath string, order []string) error {
	content, ok := readFile(root, entryPath)
	if !ok || ParseFrontmatter(content).Status == nil {
		return errf(`No entry found for "%s"`, entryPath)
	}
	if err := write(root, entryPath, SetFrontmatterPlanOrder(content, order)); err != nil {
		return err
	}
	WriteShim(root)
	return nil
}

// BackfillSubtasks sets subtask_of on every plan file that shares a folder with a plan.md.
func BackfillSubtasks(root string) []string {
	updated := []string{}
	files := WalkPlanFiles(root)
	folders := planMdFolders(files)
	for _, rel := range files {
		dir := path.Dir(rel)
		if path.Base(rel) == "plan.md" || !folders[dir] {
			continue
		}
		content, _ := readFile(root, rel)
		fm := ParseFrontmatter(content)
		if fm.Informational {
			continue
		}
		expected := dir + "/plan.md"
		if fm.SubtaskOf != expected {
			status := "Not Started"
			if fm.Status != nil && *fm.Status != "" {
				status = *fm.Status
			}
			if write(root, rel, SetFrontmatterSubtask(content, status, expected)) == nil {
				updated = append(updated, rel)
			}
		}
	}
	if len(updated) > 0 {
		WriteShim(root)
	}
	return updated
}

// EnsureFrontmatter adds "Not Started" frontmatter to any plan file lacking a status.
func EnsureFrontmatter(root string) []string {
	updated := []string{}
	files := WalkPlanFiles(root)
	folders := planMdFolders(files)
	for _, rel := range files {
		content, _ := readFile(root, rel)
		fm := ParseFrontmatter(content)
		if fm.Informational || fm.Status != nil {
			continue
		}
		var out string
		if path.Base(rel) != "plan.md" && fm.SubtaskOf == "" && folders[path.Dir(rel)] {
			out = SetFrontmatterSubtask(content, "Not Started", path.Dir(rel)+"/plan.md")
		} else {
			out = SetFrontmatterStatus(content, "Not Started")
		}
		if write(root, rel, out) == nil {
			updated = append(updated, rel)
		}
	}
	if len(updated) > 0 {
		WriteShim(root)
	}
	return updated
}

// HasOpenBacklog reports whether anything is Not Started or In Progress.
func HasOpenBacklog(entries []Entry) bool {
	for _, e := range entries {
		if e.Status == "Not Started" || e.Status == "In Progress" {
			return true
		}
	}
	return false
}

// SetBody replaces everything after a plan file's frontmatter block, leaving the block as is.
// The plan must already carry a status.
func SetBody(root, entryPath, body string) error {
	content, ok := readFile(root, entryPath)
	if !ok || ParseFrontmatter(content).Status == nil {
		return errf(`No entry found for "%s"`, entryPath)
	}
	return write(root, entryPath, SetFrontmatterBody(content, body))
}

// ReadEntryStatus returns a plan file's status; ok is false when the file is missing or has none.
func ReadEntryStatus(root, entryPath string) (status string, ok bool) {
	content, found := readFile(root, entryPath)
	if !found {
		return "", false
	}
	fm := ParseFrontmatter(content)
	if fm.Status == nil {
		return "", false
	}
	return *fm.Status, true
}

// ReadBody returns the text after a plan file's frontmatter block.
func ReadBody(root, entryPath string) (string, bool) {
	content, ok := readFile(root, entryPath)
	if !ok {
		return "", false
	}
	return ParseFrontmatter(content).Body, true
}
