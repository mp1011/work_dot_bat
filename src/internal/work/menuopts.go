// Package work is the interactive launcher: the Step 2 action menu for a chosen plan, launching
// the configured agent on it while holding the plan/branch claim, and the small human-only
// workflows (commit, cancel, mark done, set auto).
package work

import "work_dot_bat/src/internal/planstatus"

// Option is one entry of the action menu.
type Option struct{ Key, Label string }

// TaskMenuOptions is the Step 2 menu for task (nil = no task selected). Availability depends on
// the task's status, whether its branch has a worktree, and whether it has subtasks.
func TaskMenuOptions(task *planstatus.Entry, hasWorktree, isParent bool) (actions, navigational []Option) {
	if task == nil {
		actions = []Option{
			{"c", "Commit changes"},
			{"p", "Push changes"},
			{"t", "Run automated tests"},
		}
		return actions, []Option{{"s", "Select task"}, {"q", "Quit"}}
	}
	label := "Work on Task"
	if isParent {
		label = "Work on Next Subtask"
	}
	actions = append(actions, Option{"w", label})
	if isParent {
		actions = append(actions, Option{"l", "Loop until done"})
	}
	actions = append(actions, Option{"g", "General agent request"})
	if task.Status == "Ready for Review" {
		actions = append(actions, Option{"d", "Mark task as Done"})
	}
	if task.Status != "Done" && task.Status != "Cancelled" {
		actions = append(actions, Option{"c", "Cancel task"})
	}
	if hasWorktree && (task.Status == "Ready for Review" || task.Status == "Done") {
		actions = append(actions, Option{"m", "Merge back into main"})
	}
	actions = append(actions, Option{"t", "Run automated tests"})
	if (task.Status == "Not Started" || task.Status == "Suggestion") && !task.Auto {
		actions = append(actions, Option{"a", "Set auto: true (commits)"})
	}
	return actions, []Option{{"s", "Start new task"}, {"q", "Quit"}}
}

// TestMenuOptions returns the second-tier menu options for choosing which test level to run.
func TestMenuOptions() (actions, navigational []Option) {
	actions = []Option{
		{"u", "Run ultra-light tests"},
		{"1", "Run light tests"},
		{"2", "Run heavy tests"},
		{"3", "Run full pre-push test gate"},
	}
	return actions, []Option{{"b", "Back"}}
}

