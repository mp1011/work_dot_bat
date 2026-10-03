// Package agentsession names agent runs, records them in the session ledger, and classifies how a
// run ended (completed, failed, or bounced off a usage limit) from its transcript.
package agentsession

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

const DefaultNamespace = "7a4e6988-92f4-42b7-8ccb-a3d850eb2617"

// CleanPlanSlug is the filename-safe form of a plan path used in session names.
func CleanPlanSlug(planPath string) string {
	if planPath == "" {
		return ""
	}
	n := strings.ReplaceAll(planPath, `\`, "/")
	n = slugPrefixRE.ReplaceAllString(n, "")
	n = strings.TrimSuffix(n, ".md")
	n = strings.ReplaceAll(n, "/", "__")
	return slugBadRE.ReplaceAllString(n, "_")
}

var (
	slugPrefixRE = regexp.MustCompile(`^(agent_docs/plans/|plans/|agent_docs/)`)
	slugBadRE    = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// FormatTimestamp is local time as YYYY-MM-DD_HH-MM-SS.
func FormatTimestamp(t time.Time) string { return t.Format("2006-01-02_15-04-05") }

// FormatSessionName is "<timestamp>__<slug>", plus "_attempt-N" when attempt is non-nil.
func FormatSessionName(planPath string, t time.Time, attempt *int) string {
	name := FormatTimestamp(t) + "__" + CleanPlanSlug(planPath)
	if attempt != nil {
		name += fmt.Sprintf("_attempt-%d", *attempt)
	}
	return name
}

func Attempt(n int) *int { return &n }

// DeterministicSessionID is the RFC 4122 version 5 UUID of name under namespace.
func DeterministicSessionID(name, namespace string) string {
	ns, _ := hex.DecodeString(strings.ReplaceAll(namespace, "-", ""))
	h := sha1.New()
	h.Write(ns)
	h.Write([]byte(name))
	sum := h.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	x := hex.EncodeToString(sum[:16])
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32]
}

// SessionID is DeterministicSessionID under the default namespace.
func SessionID(name string) string { return DeterministicSessionID(name, DefaultNamespace) }

// Entry is one ledger row. Rows are kept as open maps so fields written by other tools survive.
type Entry = map[string]any

const isoLayout = "2006-01-02T15:04:05.000Z"

func ISO(t time.Time) string { return t.UTC().Format(isoLayout) }

// ReadLedger returns the ledger rows; a missing or unparseable file reads as empty.
func ReadLedger(file string) []Entry {
	b, err := os.ReadFile(file)
	if err != nil {
		return []Entry{}
	}
	var ledger []Entry
	if json.Unmarshal(b, &ledger) != nil || ledger == nil {
		return []Entry{}
	}
	return ledger
}

// RecordStart appends an in-progress row for the entry, stamping pid and startedAt if absent.
func RecordStart(file string, entry Entry, pid int, now time.Time) (Entry, error) {
	rec := Entry{}
	for k, v := range entry {
		rec[k] = v
	}
	if _, ok := rec["pid"]; !ok {
		rec["pid"] = pid
	}
	if s, _ := rec["startedAt"].(string); s == "" {
		rec["startedAt"] = ISO(now)
	}
	if s, _ := rec["status"].(string); s == "" {
		rec["status"] = "in-progress"
	}
	row := NewRow()
	row.SetAll(rec)
	return rec, WriteRows(file, append(ReadRows(file), row))
}

// RecordEnd merges update into the latest row with the same sessionName (or appends one) and
// stamps endedAt. Existing rows keep their key order.
func RecordEnd(file string, update Entry, now time.Time) (Entry, error) {
	rows := ReadRows(file)
	var target *Row
	for i := len(rows) - 1; i >= 0; i-- {
		if v, _ := rows[i].Get("sessionName"); v == update["sessionName"] {
			target = rows[i]
			break
		}
	}
	if target == nil {
		target = NewRow()
		rows = append(rows, target)
	}
	target.SetAll(update)
	if target.Str("endedAt") == "" {
		target.Set("endedAt", ISO(now))
	}
	return target.Entry(), WriteRows(file, rows)
}

// FindSessionsForPlan returns the ledger rows recorded for a plan.
func FindSessionsForPlan(file, planPath string) []Entry {
	slug := CleanPlanSlug(planPath)
	var out []Entry
	for _, e := range ReadLedger(file) {
		if p, _ := e["plan"].(string); p != "" && CleanPlanSlug(p) == slug {
			out = append(out, e)
		}
	}
	return out
}
