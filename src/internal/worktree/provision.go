package worktree

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/planstatus"
	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/repo"
)

// Provisioning is the slice of a project's config a worktree needs.
type Provisioning struct {
	Install           *projectconfig.Command `json:"install"`
	DevServer         *projectconfig.Command `json:"devServer"`
	WorktreeCopyFiles []string               `json:"worktreeCopyFiles"`
}

// LoadProvisioning reads the selected project's config; a worktree's checkout only has the
// file once committed, so it falls back to the project root's copy.
func LoadProvisioning(treeRoot, projectRoot string) (Provisioning, error) {
	raw, err := projectconfig.ForProject(treeRoot, projectRoot)
	if err != nil {
		return Provisioning{}, err
	}
	b, _ := json.Marshal(raw)
	var p Provisioning
	err = json.Unmarshal(b, &p)
	return p, err
}

// NeedsShell honors an explicit `shell: true` from the config. Node needed a shell for the
// npm/npx shims on Windows; Go execs them directly (resolving npm.cmd), so only an explicit
// opt-in is kept.
func NeedsShell(c projectconfig.Command) bool {
	return c.Shell != nil && *c.Shell
}

// Bootstrap makes a fresh worktree usable: the install command, the plan_status.js shim, then
// the project's untracked machine-local files copied from the main checkout (never
// overwriting). Returns false when a required step failed.
func Bootstrap(worktreePath string, p Provisioning, run Runner, log func(string)) bool {
	if p.Install != nil {
		line := projectconfig.FormatCommand(*p.Install)
		log("Running " + line + " (this can take a few minutes)...")
		code, err := run(p.Install.Cmd, p.Install.Args, RunOpts{Dir: worktreePath, Env: p.Install.Env, Shell: NeedsShell(*p.Install)})
		if err != nil || code != 0 {
			log("[ERROR] " + line + " failed with exit code " + itoa(code))
			return false
		}
	}

	log("Regenerating agent_docs/plan_status.js...")
	planstatus.WriteShim(worktreePath)

	// Best-effort: the files are conveniences, not requirements.
	if main := repo.FindMainRepoRoot(worktreePath); main != "" && !samePath(main, worktreePath) {
		for _, rel := range p.WorktreeCopyFiles {
			src, dest := filepath.Join(main, rel), filepath.Join(worktreePath, rel)
			if _, err := os.Stat(src); err != nil {
				continue
			}
			if _, err := os.Stat(dest); err == nil {
				continue
			}
			if os.MkdirAll(filepath.Dir(dest), 0o755) != nil {
				continue
			}
			if b, err := os.ReadFile(src); err == nil && os.WriteFile(dest, b, 0o644) == nil {
				log("Copied " + rel + " to worktree.")
			}
		}
	}
	return true
}

func samePath(a, b string) bool {
	ra, ea := filepath.Abs(a)
	rb, eb := filepath.Abs(b)
	if ea != nil || eb != nil {
		return a == b
	}
	return strings.EqualFold(filepath.Clean(ra), filepath.Clean(rb))
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// StartDevServer starts the project's dev server detached in the worktree and returns its
// pid; a project without one starts nothing (pid 0).
func StartDevServer(worktreePath string, p Provisioning, log func(string)) (int, error) {
	dev := p.DevServer
	if dev == nil {
		return 0, nil
	}
	log("Starting dev server (" + projectconfig.FormatCommand(*dev) + ")...")
	cmd := exec.Command(dev.Cmd, dev.Args...)
	cmd.Dir = worktreePath
	if len(dev.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range dev.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}
