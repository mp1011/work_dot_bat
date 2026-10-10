package managed

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestFile is the catalogue's name inside the vendor tree.
const ManifestFile = "manifest.json"

// Entry records one vendored file as last released.
type Entry struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	SHA256  string `json:"sha256"`
}

// Manifest is vendor/manifest.json: each file's version, date and body hash. A changed file
// without a version bump is drift; `vendor-bump` records the change.
type Manifest struct {
	Files map[string]Entry `json:"files"`
}

// ListVendorFiles is every file that gets copied into target repos (target-layout paths, sorted).
// Tests and the manifest itself are not shipped.
func ListVendorFiles(fsys fs.FS) []string {
	var out []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == ManifestFile && path.Dir(p) == "." || strings.HasSuffix(d.Name(), ".test.ts") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	sort.Strings(out)
	return out
}

func ReadBody(fsys fs.FS, file string) (string, error) {
	b, err := fs.ReadFile(fsys, file)
	return string(b), err
}

// ReadManifest returns an empty manifest when there is none.
func ReadManifest(fsys fs.FS) (Manifest, error) {
	b, err := fs.ReadFile(fsys, ManifestFile)
	if err != nil {
		return Manifest{Files: map[string]Entry{}}, nil
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, err
	}
	if m.Files == nil {
		m.Files = map[string]Entry{}
	}
	return m, nil
}

// WriteManifest writes dir/manifest.json (keys sorted, two-space indent, trailing newline).
func WriteManifest(dir string, m Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ManifestFile), append(b, '\n'), 0o644)
}

// Drift is how the files on disk differ from the manifest.
type Drift struct{ Added, Changed, Removed []string }

func (d Drift) Clean() bool { return len(d.Added) == 0 && len(d.Changed) == 0 && len(d.Removed) == 0 }

func FindDrift(m Manifest, fsys fs.FS) (Drift, error) {
	var d Drift
	onDisk := map[string]bool{}
	for _, file := range ListVendorFiles(fsys) {
		onDisk[file] = true
		entry, ok := m.Files[file]
		if !ok {
			d.Added = append(d.Added, file)
			continue
		}
		body, err := ReadBody(fsys, file)
		if err != nil {
			return d, err
		}
		if entry.SHA256 != HashBody(body) {
			d.Changed = append(d.Changed, file)
		}
	}
	for file := range m.Files {
		if !onDisk[file] {
			d.Removed = append(d.Removed, file)
		}
	}
	sort.Strings(d.Removed)
	return d, nil
}

// ApplyBump is the manifest after recording the drift: new files start at 1.0.0, changed files
// get level bumped, removed files are dropped.
func ApplyBump(m Manifest, d Drift, level, today string, hashOf func(file string) string) (Manifest, error) {
	files := map[string]Entry{}
	for k, v := range m.Files {
		files[k] = v
	}
	for _, f := range d.Added {
		files[f] = Entry{Version: "1.0.0", Date: today, SHA256: hashOf(f)}
	}
	for _, f := range d.Changed {
		v, err := BumpVersion(files[f].Version, level)
		if err != nil {
			return m, err
		}
		files[f] = Entry{Version: v, Date: today, SHA256: hashOf(f)}
	}
	for _, f := range d.Removed {
		delete(files, f)
	}
	return Manifest{Files: files}, nil
}
