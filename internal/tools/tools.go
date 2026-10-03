// Package tools is the registry of launcher tools. Later subtasks plug ported tools in with
// Register; every name in Names is a known tool, and one without a Run is "not yet ported".
package tools

import (
	"fmt"
	"sort"
)

// Run executes a tool against the already-activated project and returns the process exit code.
type Run func(args []string) int

// Names mirrors the Node launcher's TOOLS table.
var Names = []string{
	"work", "auto_daemon", "prepare_for_push", "create_worktree", "auto_status",
	"analyze_session", "reclassify_sessions", "migrate_state", "sync", "rewrite_paths", "bootstrap",
}

var registry = map[string]Run{}

// Register plugs a ported tool in. It panics on an unknown or duplicate name (a programming error).
func Register(name string, run Run) {
	if !Known(name) {
		panic(fmt.Sprintf("tools: unknown tool %q", name))
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("tools: %q registered twice", name))
	}
	registry[name] = run
}

func Known(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

// Lookup returns the tool's Run, or nil when it is known but not yet ported.
func Lookup(name string) Run { return registry[name] }

func Registered() []string {
	var out []string
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Replace swaps a tool's Run (for tests) and returns a function that restores the original.
func Replace(name string, run Run) (restore func()) {
	prev, had := registry[name]
	registry[name] = run
	return func() {
		if had {
			registry[name] = prev
		} else {
			delete(registry, name)
		}
	}
}
