// Package planclaims is the live-work claim registry: "is anyone working on this right now?"
// Claims live at <repo>/.work_dot_bat/claims/<scope>--<slug>.json, per machine and gitignored.
// The JSON format is shared with the Node implementation in both directions.
package planclaims

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"work_dot_bat/internal/repo"
)

const (
	DaemonTTL = 30 * time.Minute // the daemon renews every attempt
	AgentTTL  = 4 * time.Hour    // an interactive agent may never renew
	// The OS reports a start time at different precisions, so two readings of one process differ.
	StartTimeTolerance = 2 * time.Second
	timeLayout         = "2006-01-02T15:04:05.000Z"
)

type Owner struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	PID       int    `json:"pid"`
	Host      string `json:"host"`
	StartedAt string `json:"startedAt,omitempty"`
}

type Claim struct {
	Scope      string  `json:"scope"`
	ID         string  `json:"id"`
	Owner      *Owner  `json:"owner"`
	Branch     string  `json:"branch"`
	Worktree   *string `json:"worktree"`
	AcquiredAt string  `json:"acquiredAt"`
	RenewedAt  string  `json:"renewedAt,omitempty"`
	TTLMs      int64   `json:"ttlMs"`
}

func (c Claim) ttl() time.Duration { return time.Duration(c.TTLMs) * time.Millisecond }

// Since is when the claim was last renewed (or acquired).
func (c Claim) Since() string {
	if c.RenewedAt != "" {
		return c.RenewedAt
	}
	return c.AcquiredAt
}

// State is the claim verdict.
type State string

const (
	Free             State = "free"
	Held             State = "held"
	StaleReclaimable State = "stale-reclaimable"
	StaleReport      State = "stale-report"
)

type Decision struct {
	State  State
	Reason string
}

// Env holds everything non-deterministic so tests can pin it.
type Env struct {
	Now            func() time.Time
	Hostname       string
	PIDAlive       func(pid int) bool
	StartTimeOf    func(pid int) (time.Time, bool)
	WorktreeExists func(path string) bool
	PID            int
}

func DefaultEnv() Env {
	host, _ := os.Hostname()
	return Env{
		Now:         time.Now,
		Hostname:    host,
		PIDAlive:    pidAlive,
		StartTimeOf: processStartedAt,
		WorktreeExists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		PID: os.Getpid(),
	}
}

var (
	slugBadRE   = regexp.MustCompile(`[^a-z0-9._-]+`)
	slugDashRE  = regexp.MustCompile(`-+`)
	slugEdgeRE  = regexp.MustCompile(`^-|-$`)
	plansPrefix = regexp.MustCompile(`^plans/`)
	mdSuffix    = regexp.MustCompile(`\.md$`)
)

// CleanPlanSlug is a plan path without agent_docs/, plans/ or .md (ledger matching).
func CleanPlanSlug(planPath string) string {
	if planPath == "" {
		return ""
	}
	s := strings.ReplaceAll(planPath, `\`, "/")
	s = strings.TrimPrefix(s, "agent_docs/")
	s = plansPrefix.ReplaceAllString(s, "")
	return mdSuffix.ReplaceAllString(s, "")
}

// ClaimFileName is deterministic, collision-resistant and filesystem-safe.
func ClaimFileName(scope, id string) string {
	slug := strings.ReplaceAll(strings.ReplaceAll(id, `\`, "/"), "/", "__")
	slug = slugBadRE.ReplaceAllString(strings.ToLower(slug), "-")
	slug = slugDashRE.ReplaceAllString(slug, "-")
	slug = slugEdgeRE.ReplaceAllString(slug, "")
	return scope + "--" + slug + ".json"
}

// FormatAge is the compact age shared with the CLI and list-tasks.
func FormatAge(d time.Duration) string {
	sec := int64(math.Floor(d.Seconds()))
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	if min := sec / 60; min < 60 {
		return fmt.Sprintf("%dm", min)
	} else if hr := min / 60; hr < 24 {
		return fmt.Sprintf("%dh", hr)
	} else {
		return fmt.Sprintf("%dd", hr/24)
	}
}

func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

// IsSameProcess: either time missing or unparseable means "cannot tell", which stays
// conservative and treats the pid as the same owner.
func IsSameProcess(recordedStartedAt string, actual time.Time, actualKnown bool) bool {
	if recordedStartedAt == "" || !actualKnown {
		return true
	}
	rec, ok := parseTime(recordedStartedAt)
	if !ok {
		return true
	}
	d := rec.Sub(actual)
	if d < 0 {
		d = -d
	}
	return d <= StartTimeTolerance
}

func DescribeOwner(o *Owner) string {
	if o == nil {
		return "unknown owner"
	}
	return fmt.Sprintf(`%s "%s" (pid %d, %s)`, o.Kind, o.Label, o.PID, o.Host)
}

// Liveness: nil means "could not be determined".
type Liveness struct {
	PIDAlive       *bool
	WorktreeExists *bool
}

func boolp(b bool) *bool { return &b }

// ClaimDecision is deliberately conservative: only a provably dead pid or a provably missing
// worktree is reclaimable; everything else past TTL is reported, never silently stolen.
func ClaimDecision(existing *Claim, now time.Time, lv Liveness) Decision {
	if existing == nil {
		return Decision{Free, "no existing claim"}
	}
	age := time.Duration(math.MaxInt64)
	if since, ok := parseTime(existing.Since()); ok {
		age = now.Sub(since)
	}
	dead := lv.PIDAlive != nil && !*lv.PIDAlive
	kind := ""
	if existing.Owner != nil {
		kind = existing.Owner.Kind
	}
	pidReason := func() string {
		pid, host := 0, ""
		if existing.Owner != nil {
			pid, host = existing.Owner.PID, existing.Owner.Host
		}
		return fmt.Sprintf("owner pid %d is not running on %s", pid, host)
	}
	// A crashed daemon/launcher holds its claim with a long-lived pid, so a dead pid is
	// reclaimable before TTL. Agent claims record an ephemeral CLI pid and stay TTL-bound.
	if dead && (kind == "daemon" || kind == "launcher") {
		return Decision{StaleReclaimable, pidReason()}
	}
	if age < existing.ttl() {
		return Decision{Held, fmt.Sprintf("held by %s, %s ago", DescribeOwner(existing.Owner), FormatAge(age))}
	}
	if dead {
		return Decision{StaleReclaimable, pidReason()}
	}
	if lv.WorktreeExists != nil && !*lv.WorktreeExists {
		wt := ""
		if existing.Worktree != nil {
			wt = *existing.Worktree
		}
		return Decision{StaleReclaimable, fmt.Sprintf("worktree %s no longer exists", wt)}
	}
	return Decision{StaleReport, fmt.Sprintf("past ttl (%s) — owner may still be running; use --steal to override", FormatAge(age))}
}

// BlockingClaim is a held or stale-report claim.
type BlockingClaim struct {
	Record   Claim
	Decision Decision
}

// ClassifyConcurrentWork turns blocking claims plus a worktree branch that advanced the plan
// into the reasons a fresh run should not start. No reasons means go.
func ClassifyConcurrentWork(blocking []BlockingClaim, worktreeBranch, worktreeStatus string) (free bool, reasons []string) {
	for _, c := range blocking {
		who := DescribeOwner(c.Record.Owner)
		if c.Decision.State == StaleReport {
			reasons = append(reasons, fmt.Sprintf("stale claim by %s — %s", who, c.Decision.Reason))
		} else {
			reasons = append(reasons, "live claim — "+c.Decision.Reason)
		}
	}
	if worktreeBranch != "" {
		movedTo := "tracked only on"
		if worktreeStatus != "" && worktreeStatus != "Not Started" {
			movedTo = fmt.Sprintf("advanced to \"%s\" on", worktreeStatus)
		}
		reasons = append(reasons, fmt.Sprintf("%s worktree branch '%s' — unmerged work exists there", movedTo, worktreeBranch))
	}
	return len(reasons) == 0, reasons
}

// ---------------------------------------------------------------------------
// I/O
// ---------------------------------------------------------------------------

func claimFilePath(root, scope, id string) string {
	return filepath.Join(repo.ClaimsDir(root), ClaimFileName(scope, id))
}

// readClaimRaw treats a missing, corrupt or incomplete file as no claim.
func readClaimRaw(p string) *Claim {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var c Claim
	if json.Unmarshal(b, &c) != nil || c.Owner == nil || c.AcquiredAt == "" || c.TTLMs == 0 {
		return nil
	}
	return &c
}

func marshalClaim(c Claim) []byte {
	b, _ := json.MarshalIndent(c, "", "  ")
	return append(b, '\n')
}

func (e Env) liveness(c *Claim) Liveness {
	var lv Liveness
	if c == nil {
		return lv
	}
	if c.Owner != nil && c.Owner.Host == e.Hostname {
		alive := e.PIDAlive(c.Owner.PID)
		// The pid answers, but is it still the owner? A reused pid belongs to another process.
		if alive && c.Owner.StartedAt != "" {
			t, ok := e.StartTimeOf(c.Owner.PID)
			alive = IsSameProcess(c.Owner.StartedAt, t, ok)
		}
		lv.PIDAlive = boolp(alive)
	}
	if c.Worktree != nil && *c.Worktree != "" {
		lv.WorktreeExists = boolp(e.WorktreeExists(*c.Worktree))
	}
	return lv
}

func (e Env) withStartTime(o Owner) Owner {
	if o.StartedAt != "" || o.Kind == "agent" || o.Host != e.Hostname {
		return o
	}
	if t, ok := e.StartTimeOf(o.PID); ok {
		o.StartedAt = formatTime(t)
	}
	return o
}

type Scope struct {
	Scope, ID, Branch string
	Worktree          string // "" = none
}

func tryWrite(p string, c Claim) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, err
	}
	_, werr := f.Write(marshalClaim(c))
	cerr := f.Close()
	if werr != nil {
		return false, werr
	}
	return true, cerr
}

func unlinkIgnoreMissing(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func rollback(paths []string) {
	for _, p := range paths {
		unlinkIgnoreMissing(p)
	}
}

type AcquireResult struct {
	OK           bool
	Claims       []Claim
	BlockedScope string
	BlockedID    string
	Decision     Decision
}

// Acquire takes every scope or none: a held or stale-report scope blocks the whole call and
// rolls back what this call already wrote. steal forces every scope.
func (e Env) Acquire(root string, scopes []Scope, requested Owner, ttl time.Duration, steal bool) (AcquireResult, error) {
	now := e.Now()
	nowIso := formatTime(now)
	owner := e.withStartTime(requested)
	var written []string

	for _, s := range scopes {
		p := claimFilePath(root, s.Scope, s.ID)
		var wt *string
		if s.Worktree != "" {
			w := s.Worktree
			wt = &w
		}
		own := owner
		rec := Claim{Scope: s.Scope, ID: s.ID, Owner: &own, Branch: s.Branch, Worktree: wt,
			AcquiredAt: nowIso, RenewedAt: nowIso, TTLMs: ttl.Milliseconds()}

		ok, err := tryWrite(p, rec)
		if err != nil {
			rollback(written)
			return AcquireResult{}, err
		}
		if ok {
			written = append(written, p)
			continue
		}

		existing := readClaimRaw(p)
		if existing == nil {
			if _, err := os.Stat(p); err == nil {
				// In-flight write race: another process just created the file but not flushed it.
				for attempt := 0; attempt < 10 && existing == nil; attempt++ {
					time.Sleep(10 * time.Millisecond)
					existing = readClaimRaw(p)
				}
			}
		}
		decision := ClaimDecision(existing, now, e.liveness(existing))
		reclaimable := steal || decision.State == Free || decision.State == StaleReclaimable
		if !reclaimable {
			rollback(written)
			return AcquireResult{BlockedScope: s.Scope, BlockedID: s.ID, Decision: decision}, nil
		}

		if err := unlinkIgnoreMissing(p); err != nil {
			rollback(written)
			return AcquireResult{}, err
		}
		ok, err = tryWrite(p, rec)
		if err != nil {
			rollback(written)
			return AcquireResult{}, err
		}
		if !ok {
			// Lost a race against a third writer between the unlink and the retry.
			raced := readClaimRaw(p)
			rollback(written)
			return AcquireResult{BlockedScope: s.Scope, BlockedID: s.ID,
				Decision: ClaimDecision(raced, now, e.liveness(raced))}, nil
		}
		written = append(written, p)
	}

	res := AcquireResult{OK: true}
	for _, p := range written {
		if c := readClaimRaw(p); c != nil {
			res.Claims = append(res.Claims, *c)
		}
	}
	return res, nil
}

// Release removes the claim file; releasing something not held is a no-op.
func Release(root, scope, id string) error {
	return unlinkIgnoreMissing(claimFilePath(root, scope, id))
}

// Renew rewrites an existing claim with a fresh renewedAt; it fails if nothing is held.
func (e Env) Renew(root, scope, id string) (Claim, error) {
	p := claimFilePath(root, scope, id)
	c := readClaimRaw(p)
	if c == nil {
		return Claim{}, fmt.Errorf("Cannot renew %s:%s — no claim held (already released or never acquired)", scope, id)
	}
	c.RenewedAt = formatTime(e.Now())
	return *c, os.WriteFile(p, marshalClaim(*c), 0o644)
}

func ReadClaim(root, scope, id string) *Claim { return readClaimRaw(claimFilePath(root, scope, id)) }

type Found struct {
	Record   Claim
	Decision Decision
}

// ReadClaimWithDecision is read-only: the record plus its decision, or nil if nothing is claimed.
func (e Env) ReadClaimWithDecision(root, scope, id string) *Found {
	c := readClaimRaw(claimFilePath(root, scope, id))
	if c == nil {
		return nil
	}
	return &Found{*c, ClaimDecision(c, e.Now(), e.liveness(c))}
}

type Listed struct {
	Claim
	Decision Decision
}

func claimFiles(root string) []string {
	entries, err := os.ReadDir(repo.ClaimsDir(root))
	if err != nil {
		return nil
	}
	var out []string
	for _, ent := range entries {
		if strings.HasSuffix(ent.Name(), ".json") {
			out = append(out, filepath.Join(repo.ClaimsDir(root), ent.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// ListClaims returns every parsed claim annotated with its decision.
func (e Env) ListClaims(root string) []Listed {
	now := e.Now()
	var out []Listed
	for _, p := range claimFiles(root) {
		if c := readClaimRaw(p); c != nil {
			out = append(out, Listed{*c, ClaimDecision(c, now, e.liveness(c))})
		}
	}
	return out
}
