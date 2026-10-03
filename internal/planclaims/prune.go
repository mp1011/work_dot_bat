package planclaims

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"work_dot_bat/internal/repo"
)

type PruneResult struct {
	Removed []Listed
	Kept    []Listed
}

// markLedgerRowAbandoned flips matching in-progress ledger rows to abandoned. Best effort:
// any read/parse/write problem is ignored, and unknown row fields are preserved.
func markLedgerRowAbandoned(ledgerFile, plan string, ownerPID int, nowIso string) {
	b, err := os.ReadFile(ledgerFile)
	if err != nil {
		return
	}
	var ledger []map[string]any
	if json.Unmarshal(b, &ledger) != nil {
		return
	}
	slug := CleanPlanSlug(plan)
	modified := false
	for _, entry := range ledger {
		entryPlan, _ := entry["plan"].(string)
		if entry["status"] != "in-progress" || CleanPlanSlug(entryPlan) != slug {
			continue
		}
		pid, has := entry["pid"]
		if !has || pid == nil || fmt.Sprint(pid) == fmt.Sprint(ownerPID) {
			entry["status"] = "abandoned"
			entry["endedAt"] = nowIso
			modified = true
		}
	}
	if !modified {
		return
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if enc.Encode(ledger) == nil {
		os.WriteFile(ledgerFile, buf.Bytes(), 0o644)
	}
}

func resolveLedger(root, ledgerFile string) string {
	if ledgerFile != "" {
		return ledgerFile
	}
	resolved := repo.LedgerPath(root)
	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		common := strings.TrimSpace(string(out))
		if !filepath.IsAbs(common) {
			common = filepath.Join(root, common)
		}
		main := repo.LedgerPath(filepath.Dir(filepath.Clean(common)))
		if _, err := os.Stat(main); err == nil {
			return main
		}
	}
	return resolved
}

// Prune removes every claim judged stale-reclaimable and marks the ledger rows of dead
// launcher/daemon owners abandoned. Held and stale-report claims are kept (the latter is
// reported, never removed). ledgerFile "" means the project's ledger.
func (e Env) Prune(root, ledgerFile string) PruneResult {
	var res PruneResult
	if _, err := os.Stat(repo.ClaimsDir(root)); err != nil {
		return res
	}
	ledger := resolveLedger(root, ledgerFile)
	now := e.Now()
	nowIso := formatTime(now)
	for _, p := range claimFiles(root) {
		rec := readClaimRaw(p)
		if rec == nil {
			continue
		}
		decision := ClaimDecision(rec, now, e.liveness(rec))
		item := Listed{*rec, decision}
		if decision.State != StaleReclaimable {
			res.Kept = append(res.Kept, item)
			continue
		}
		// Only delete what we judged: skip if acquire() rewrote this file since we read it.
		cur := readClaimRaw(p)
		if cur == nil || cur.AcquiredAt != rec.AcquiredAt || cur.RenewedAt != rec.RenewedAt {
			continue
		}
		unlinkIgnoreMissing(p)
		res.Removed = append(res.Removed, item)
		if rec.Scope == "plan" && rec.Owner != nil && (rec.Owner.Kind == "launcher" || rec.Owner.Kind == "daemon") {
			markLedgerRowAbandoned(ledger, rec.ID, rec.Owner.PID, nowIso)
		}
	}
	return res
}

// PruneStaleClaims is the best-effort sweep every launcher runs at startup: a failed sweep
// must never keep a launcher from starting.
func (e Env) PruneStaleClaims(root string, log func(string)) []Listed {
	res := e.Prune(root, "")
	if len(res.Removed) > 0 {
		log(fmt.Sprintf("Pruned %d stale claim(s) left by earlier runs:", len(res.Removed)))
		for _, c := range res.Removed {
			log(fmt.Sprintf("  %s %s — %s", c.Scope, c.ID, c.Decision.Reason))
		}
	}
	return res.Removed
}
