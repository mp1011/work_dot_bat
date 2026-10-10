// Package statemigrate is the one-time move of a project's runtime state from the old locations
// (.tools/session_ledger.json, .tools/logs/, .claims/) into the single .work_dot_bat/ folder.
// A dry run unless the caller applies the plan.
package statemigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/repo"
)

type Move struct{ From, To string }
type Skip struct{ From, Reason string }

type Plan struct {
	Moves   []Move
	Skipped []Skip
	Dead    []string
}

// Legacy state the daemon no longer reads or writes.
var deadState = []string{".tools/auto_daemon_state.json"}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return filepath.ToSlash(r)
}

func PlanMigration(root string) Plan {
	var plan Plan
	for _, c := range []struct{ rel, to string }{
		{".tools/session_ledger.json", repo.LedgerPath(root)},
		{".tools/logs", repo.LogsDir(root)},
		{".claims", repo.ClaimsDir(root)},
	} {
		from := filepath.Join(root, filepath.FromSlash(c.rel))
		if !exists(from) {
			continue
		}
		if exists(c.to) {
			plan.Skipped = append(plan.Skipped, Skip{from, rel(root, c.to) + " already exists — left in place"})
			continue
		}
		plan.Moves = append(plan.Moves, Move{from, c.to})
	}
	for _, d := range deadState {
		if p := filepath.Join(root, filepath.FromSlash(d)); exists(p) {
			plan.Dead = append(plan.Dead, p)
		}
	}
	return plan
}

// Apply performs the moves and returns the ones it made.
func Apply(plan Plan, root string) ([]Move, error) {
	if len(plan.Moves) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(repo.StateDir(root), 0o755); err != nil {
		return nil, err
	}
	var done []Move
	for _, m := range plan.Moves {
		if err := os.Rename(m.From, m.To); err != nil {
			return done, err
		}
		done = append(done, m)
	}
	return done, nil
}

// GitignoreCoversState reports whether .gitignore already lists the state folder.
func GitignoreCoversState(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		l := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "/"), "/")
		if l == repo.StateDirName {
			return true
		}
	}
	return false
}

func FormatReport(plan Plan, applied bool, root string, ignored bool) string {
	var lines []string
	if len(plan.Moves) == 0 && len(plan.Skipped) == 0 && len(plan.Dead) == 0 {
		lines = append(lines, "Nothing to migrate: no legacy state found.")
	}
	for _, m := range plan.Moves {
		verb := "Would move"
		if applied {
			verb = "Moved"
		}
		lines = append(lines, fmt.Sprintf("%s: %s -> %s", verb, rel(root, m.From), rel(root, m.To)))
	}
	for _, s := range plan.Skipped {
		lines = append(lines, fmt.Sprintf("Skipped: %s (%s)", rel(root, s.From), s.Reason))
	}
	for _, d := range plan.Dead {
		lines = append(lines, fmt.Sprintf("Not migrated (obsolete, safe to delete): %s", rel(root, d)))
	}
	if !applied && len(plan.Moves) > 0 {
		lines = append(lines, "Dry run. Re-run with --apply to move the state. Stop any running daemon first.")
	}
	if !ignored && (applied || len(plan.Moves) > 0) {
		lines = append(lines, fmt.Sprintf("Note: add \"%s/\" to this repo's .gitignore.", repo.StateDirName))
	}
	return strings.Join(lines, "\n")
}
