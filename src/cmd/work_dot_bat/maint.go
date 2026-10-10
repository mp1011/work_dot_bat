package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	wdb "work_dot_bat"
	"work_dot_bat/src/internal/maintcli"
	"work_dot_bat/src/internal/repo"
	"work_dot_bat/src/internal/tools"
)

func maintConfig() maintcli.Config {
	root := os.Getenv(repo.ProjectRootEnv)
	if root == "" {
		root, _ = os.Getwd()
	}
	vendor, _ := fs.Sub(wdb.Vendor, "src/vendor")
	templates, _ := fs.Sub(wdb.Templates, "templates/files")
	st, err := os.Stdin.Stat()
	return maintcli.Config{
		Root: filepath.FromSlash(root), ProjectName: os.Getenv(repo.ProjectNameEnv),
		In: os.Stdin, Out: os.Stdout, Err: os.Stderr,
		Interactive: err == nil && st.Mode()&os.ModeCharDevice != 0,
		Vendor:      vendor, Templates: templates,
	}
}

func init() {
	tools.Register("sync", func(a []string) int { return maintcli.Sync(maintConfig(), a) })
	tools.Register("bootstrap", func(a []string) int { return maintcli.Bootstrap(maintConfig(), a) })
	tools.Register("migrate_state", func(a []string) int { return maintcli.MigrateState(maintConfig(), a) })
	tools.Register("rewrite_paths", func(a []string) int { return maintcli.RewritePaths(maintConfig(), a) })
}

// runMaintCommand handles the subcommands that are not project tools; ok is false for others.
func runMaintCommand(argv []string, e env) (code int, ok bool) {
	if code, ok := checksEnv().Dispatch(context.Background(), argv[0], argv[1:]); ok {
		return code, true
	}
	switch argv[0] {
	case "safety-hook":
		return maintcli.SafetyHook(e.stdin, e.stderr), true
	case "vendor-check":
		wd, _ := os.Getwd()
		return maintcli.VendorCheck(wd, e.stdout), true
	case "vendor-bump":
		wd, _ := os.Getwd()
		return maintcli.VendorBump(wd, argv[1:], time.Now(), e.stdout), true
	}
	return 0, false
}
