// Package maintcli is the command-line front end of the repo-maintenance tools: sync, bootstrap,
// migrate_state, rewrite_paths, the safety hook, and the vendor manifest check/bump used while
// developing the tool itself.
package maintcli

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"work_dot_bat/src/internal/bootstrap"
	"work_dot_bat/src/internal/managed"
	"work_dot_bat/src/internal/pathrewrite"
	"work_dot_bat/src/internal/safetyhook"
	"work_dot_bat/src/internal/statemigrate"
	"work_dot_bat/src/internal/syncer"
)

// Config is the process-level context for the tools.
type Config struct {
	Root        string
	ProjectName string
	In          io.Reader
	Out, Err    io.Writer
	// Interactive says whether stdin is a terminal that can answer questions.
	Interactive bool
	Vendor       fs.FS // the vendored files, laid out as in a target repo
	Templates    fs.FS // templates/files
	Now          func() time.Time
	RegistryFile string
}

func (c Config) today() string {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	return now().UTC().Format("2006-01-02")
}

func has(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// Sync installs/updates the vendored scripts inside the project (never commits).
//
//	--dry-run  show what would change
//	--yes      overwrite locally edited / unmanaged files without asking
//
// It exits non-zero when something that needed attention was left undone.
func Sync(c Config, args []string) int {
	dryRun, yes := has(args, "--dry-run"), has(args, "--yes")
	if !syncer.IsMainCheckout(c.Root) {
		fmt.Fprintf(c.Err, "%s is not a main checkout (it looks like a git worktree). Run sync against the main checkout; worktrees get the changes by merging.\n", c.Root)
		return 1
	}
	manifest, err := managed.ReadManifest(c.Vendor)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	plan := syncer.PlanSync(c.Root, manifest)

	answers := map[string]syncer.Decision{}
	var conflicts []syncer.Item
	for _, i := range plan.Items {
		if i.State == syncer.Modified || i.State == syncer.Unmanaged {
			conflicts = append(conflicts, i)
		}
	}
	if len(conflicts) > 0 && !yes && !dryRun && c.Interactive {
		in := bufio.NewReader(c.In)
		for _, i := range conflicts {
			why := "has no managed header"
			if i.State == syncer.Modified {
				why = "was edited locally"
			}
			fmt.Fprintf(c.Out, "%s %s. Overwrite with the tool's version %s? (y/N) ", i.File, why, i.Version)
			line, _ := in.ReadString('\n')
			if r := strings.ToLower(strings.TrimSpace(line)); r == "y" || r == "yes" {
				answers[i.File] = syncer.Overwrite
			} else {
				answers[i.File] = syncer.Skip
			}
		}
	}
	res, err := syncer.Apply(c.Root, plan, manifest, c.Vendor, syncer.Options{
		DryRun: dryRun, Today: c.today(),
		Decide: func(i syncer.Item) syncer.Decision {
			if yes {
				return syncer.Overwrite
			}
			return answers[i.File]
		},
	})
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	fmt.Fprintln(c.Out, syncer.FormatReport(plan, res, dryRun))
	if len(res.Skipped) > 0 || (dryRun && len(plan.StaleItems()) > 0) {
		return 1
	}
	return 0
}

// Bootstrap prepares the project for work_dot_bat. Never overwrites; never commits.
func Bootstrap(c Config, args []string) int {
	dryRun := has(args, "--dry-run")
	name := c.ProjectName
	if name == "" {
		name = "this project"
	}
	res, err := bootstrap.Run(c.Root, bootstrap.Options{
		ProjectName: name, DryRun: dryRun, Vendor: c.Vendor, Templates: c.Templates, Today: c.today(),
		RegistryFile: c.RegistryFile,
	})
	if err != nil {
		fmt.Fprintln(c.Err, err.Error())
		return 1
	}
	fmt.Fprintln(c.Out, bootstrap.FormatReport(res, dryRun))
	return 0
}

// MigrateState moves legacy runtime state into .work_dot_bat/ (dry run unless --apply).
func MigrateState(c Config, args []string) int {
	apply := has(args, "--apply")
	plan := statemigrate.PlanMigration(c.Root)
	if apply {
		if _, err := statemigrate.Apply(plan, c.Root); err != nil {
			fmt.Fprintln(c.Err, err)
			return 1
		}
	}
	fmt.Fprintln(c.Out, statemigrate.FormatReport(plan, apply, c.Root, statemigrate.GitignoreCoversState(c.Root)))
	return 0
}

// RewritePaths rewrites references to the old script locations into scripts/work_dot_bat/
// (dry run unless --apply). It skips agent_docs/plans/ (history) and the managed folder itself.
func RewritePaths(c Config, args []string) int {
	apply := has(args, "--apply")
	manifest, err := managed.ReadManifest(c.Vendor)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	changed, err := pathrewrite.RewriteRepo(c.Root, pathrewrite.BuildPathMap(manifest), apply, nil)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	total := 0
	for _, ch := range changed {
		verb := "Would rewrite"
		if apply {
			verb = "Rewrote"
		}
		fmt.Fprintf(c.Out, "%s: %s (%d)\n", verb, ch.File, ch.Count)
		total += ch.Count
	}
	tail := ". Dry run: re-run with --apply."
	if apply {
		tail = " rewritten."
	}
	fmt.Fprintf(c.Out, "%d file(s), %d reference(s)%s\n", len(changed), total, tail)
	return 0
}

// SafetyHook is the Claude Code PreToolUse hook: it reads the tool call as JSON on stdin and exits
// 2 (message on stderr) for commands an agent must not run on the user's behalf.
func SafetyHook(in io.Reader, errOut io.Writer) int {
	b, _ := io.ReadAll(in)
	code, msg := safetyhook.Check(b)
	if code != 0 {
		fmt.Fprintln(errOut, msg)
	}
	return code
}

func report(out io.Writer, d managed.Drift) {
	for _, f := range d.Added {
		fmt.Fprintf(out, "  new      %s\n", f)
	}
	for _, f := range d.Changed {
		fmt.Fprintf(out, "  changed  %s\n", f)
	}
	for _, f := range d.Removed {
		fmt.Fprintf(out, "  removed  %s\n", f)
	}
}

func vendorDirOf(dir string) (string, string) {
	candidate := filepath.Join(dir, "src", "vendor")
	if st, err := os.Stat(candidate); err == nil && st.IsDir() {
		return candidate, "src/vendor"
	}
	return filepath.Join(dir, "vendor"), "vendor"
}

// VendorCheck fails if any vendored file changed without a recorded version bump. dir is the
// tool checkout holding src/vendor/ or vendor/.
func VendorCheck(dir string, out io.Writer) int {
	vDir, label := vendorDirOf(dir)
	fsys := os.DirFS(vDir)
	m, err := managed.ReadManifest(fsys)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	d, err := managed.FindDrift(m, fsys)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if d.Clean() {
		fmt.Fprintf(out, "%s/manifest.json matches the vendored files.\n", label)
		return 0
	}
	fmt.Fprintf(out, "Vendored files differ from %s/manifest.json — run `work_dot_bat vendor-bump`:\n", label)
	report(out, d)
	return 1
}

// VendorBump records vendored-file changes in vendor/manifest.json: new files start at 1.0.0,
// changed ones get their patch (or --minor/--major) bumped.
func VendorBump(dir string, args []string, now time.Time, out io.Writer) int {
	vendorDir, _ := vendorDirOf(dir)
	fsys := os.DirFS(vendorDir)
	m, err := managed.ReadManifest(fsys)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	d, err := managed.FindDrift(m, fsys)
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if d.Clean() {
		fmt.Fprintln(out, "Nothing to bump.")
		return 0
	}
	level := "patch"
	if has(args, "--major") {
		level = "major"
	} else if has(args, "--minor") {
		level = "minor"
	}
	today := now.UTC().Format("2006-01-02")
	next, err := managed.ApplyBump(m, d, level, today, func(file string) string {
		body, _ := managed.ReadBody(fsys, file)
		return managed.HashBody(body)
	})
	if err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	if err := managed.WriteManifest(vendorDir, next); err != nil {
		fmt.Fprintln(out, err)
		return 1
	}
	fmt.Fprintf(out, "Recorded (%s bump, %s):\n", level, today)
	report(out, d)
	return 0
}
