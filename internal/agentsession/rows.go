package agentsession

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Row is one ledger row with its keys in file order. Rewriting a ledger another tool (or an
// earlier run) wrote must not reshuffle every row's keys, or every rewrite is a whole-file diff.
type Row struct {
	keys []string
	vals map[string]any
}

func NewRow() *Row { return &Row{vals: map[string]any{}} }

func (r *Row) Get(k string) (any, bool) { v, ok := r.vals[k]; return v, ok }

// Str is the value as a string ("" when absent or not a string).
func (r *Row) Str(k string) string { s, _ := r.vals[k].(string); return s }

// Set replaces a key in place, or appends it.
func (r *Row) Set(k string, v any) {
	if _, ok := r.vals[k]; !ok {
		r.keys = append(r.keys, k)
	}
	r.vals[k] = v
}

// Entry is the row as a plain map (order lost), for readers.
func (r *Row) Entry() Entry {
	e := Entry{}
	for k, v := range r.vals {
		e[k] = v
	}
	return e
}

// preferred is the order Node wrote new keys in, so rows written by either tool look alike.
var preferred = []string{"sessionName", "plan", "agent", "sessionId", "logFile", "pid", "startedAt", "status", "exitCode", "resetAt", "endedAt"}

// SetAll sets every key of e: known keys in the preferred order, the rest alphabetically.
func (r *Row) SetAll(e Entry) {
	done := map[string]bool{}
	for _, k := range preferred {
		if v, ok := e[k]; ok {
			r.Set(k, v)
			done[k] = true
		}
	}
	var rest []string
	for k := range e {
		if !done[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		r.Set(k, e[k])
	}
}

// ReadRows parses the ledger preserving key order; a missing or unparseable file reads as empty.
func ReadRows(file string) []*Row {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return nil
	}
	var rows []*Row
	for dec.More() {
		t, err := dec.Token()
		if err != nil || t != json.Delim('{') {
			return nil
		}
		row := NewRow()
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil
			}
			var v any
			if err := dec.Decode(&v); err != nil {
				return nil
			}
			row.Set(kt.(string), v)
		}
		if _, err := dec.Token(); err != nil { // closing }
			return nil
		}
		rows = append(rows, row)
	}
	return rows
}

func marshalPlain(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// WriteRows writes the ledger as JSON indented by two spaces, with no trailing newline.
func WriteRows(file string, rows []*Row) error {
	var compact bytes.Buffer
	compact.WriteByte('[')
	for i, r := range rows {
		if i > 0 {
			compact.WriteByte(',')
		}
		compact.WriteByte('{')
		for j, k := range r.keys {
			if j > 0 {
				compact.WriteByte(',')
			}
			compact.Write(marshalPlain(k))
			compact.WriteByte(':')
			compact.Write(marshalPlain(r.vals[k]))
		}
		compact.WriteByte('}')
	}
	compact.WriteByte(']')
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, out.Bytes(), 0o644)
}
