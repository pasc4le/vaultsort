package rules

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fileInfo holds metadata about a file being evaluated.
type fileInfo struct {
	name       string
	extension  string
	fullPath   string
	modTime    time.Time
	birthTime  time.Time
}

// gatherFileInfo collects metadata about a file for rule matching.
func gatherFileInfo(path string) (fileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileInfo{}, err
	}

	fi := fileInfo{
		name:      filepath.Base(path),
		extension: strings.TrimPrefix(filepath.Ext(path), "."),
		fullPath:  path,
		modTime:   info.ModTime(),
		birthTime: getBirthTime(info),
	}

	return fi, nil
}

// matchExtension checks if the file extension matches (case-insensitive).
func matchExtension(ext string, allowed []string) bool {
	if len(allowed) == 0 {
		return true // no extension filter means pass-through
	}
	ext = strings.ToLower(ext)
	for _, a := range allowed {
		if strings.ToLower(a) == ext {
			return true
		}
	}
	return false
}

// matchPrefix checks if the filename starts with the given prefix.
func matchPrefix(name, prefix string) bool {
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(name, prefix)
}

// matchSuffix checks if the filename (before extension) ends with the given suffix.
func matchSuffix(name, suffix string) bool {
	if suffix == "" {
		return true
	}
	// Strip extension for suffix matching
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return strings.HasSuffix(base, suffix)
}

// matchAfterTime checks if the file's time is after the given threshold.
func matchAfterTime(t time.Time, after *time.Time) bool {
	if after == nil {
		return true
	}
	return t.After(*after) || t.Equal(*after)
}

// Matches checks if a file matches all criteria of this rule.
// Returns true only if all non-zero criteria match (AND logic).
func (r *Rule) Matches(path string) (bool, error) {
	fi, err := gatherFileInfo(path)
	if err != nil {
		return false, err
	}

	if !matchExtension(fi.extension, r.Match.Extension) {
		return false, nil
	}
	if !matchPrefix(fi.name, r.Match.StartsWith) {
		return false, nil
	}
	if !matchSuffix(fi.name, r.Match.EndsWith) {
		return false, nil
	}
	if !matchAfterTime(fi.birthTime, r.Match.CreatedAfter) {
		return false, nil
	}
	if !matchAfterTime(fi.modTime, r.Match.ModifiedAfter) {
		return false, nil
	}

	return true, nil
}
