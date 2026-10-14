package checks

import (
	"path/filepath"
	"strings"

	"work_dot_bat/src/internal/projectconfig"
	"work_dot_bat/src/internal/repo"
)

// ScanConfig holds per-project scan roots and file extension filters for checks
// like monolith and duplication.
type ScanConfig struct {
	Dirs        []string
	Extensions  []string
	ExcludeDirs []string
}

var (
	defaultScanDirs    = []string{"src"}
	defaultExcludeDirs = []string{"generated", "bin", "obj"}
)

func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		Dirs:        defaultScanDirs,
		ExcludeDirs: defaultExcludeDirs,
	}
}

func normalizeExtensions(exts []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, e := range exts {
		e = strings.TrimSpace(strings.ToLower(e))
		e = strings.TrimPrefix(e, "*")
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func toStrings(list []any) []string {
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// ReadScanConfig reads scanDirs (or scanRoots), scanExtensions, and excludeDirs from the project
// configuration, falling back to defaults.
func ReadScanConfig(root string) ScanConfig {
	cfg := DefaultScanConfig()
	mainRoot := repo.FindMainRepoRoot(root)
	raw, err := projectconfig.ForProject(root, mainRoot)
	if err == nil {
		if list, ok := raw["scanDirs"].([]any); ok && len(list) > 0 {
			if s := toStrings(list); len(s) > 0 {
				cfg.Dirs = s
			}
		} else if list, ok := raw["scanRoots"].([]any); ok && len(list) > 0 {
			if s := toStrings(list); len(s) > 0 {
				cfg.Dirs = s
			}
		}
		if list, ok := raw["scanExtensions"].([]any); ok && len(list) > 0 {
			cfg.Extensions = normalizeExtensions(toStrings(list))
		}
		if list, ok := raw["excludeDirs"].([]any); ok && len(list) > 0 {
			if s := toStrings(list); len(s) > 0 {
				cfg.ExcludeDirs = s
			}
		}
	}
	return cfg
}

func (cfg ScanConfig) HasExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range cfg.Extensions {
		if ext == e {
			return true
		}
	}
	return false
}

func (cfg ScanConfig) IsExcludedDir(dirName string) bool {
	for _, d := range cfg.ExcludeDirs {
		if strings.EqualFold(dirName, d) {
			return true
		}
	}
	return false
}

func (cfg ScanConfig) IsPathExcluded(rel string) bool {
	norm := filepath.ToSlash(rel)
	parts := strings.Split(norm, "/")
	// Check directory segments (all segments before the file name)
	for i := 0; i < len(parts)-1; i++ {
		for _, d := range cfg.ExcludeDirs {
			if strings.EqualFold(parts[i], d) {
				return true
			}
		}
	}
	return false
}
