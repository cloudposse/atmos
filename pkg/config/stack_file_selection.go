package config

import (
	"path/filepath"
	"strings"
)

// SelectPhysicalStackFiles narrows an existing discovery result when stack names
// a manifest relative to the stacks directory. Otherwise it preserves all files
// so callers can resolve logical names after merging. It performs no file I/O.
func SelectPhysicalStackFiles(basePath, stack string, files []string) []string {
	if stack == "" {
		return files
	}
	var selected []string
	for _, file := range files {
		base := basePath
		if base == "" {
			base = filepath.Dir(file)
		}
		relative, err := filepath.Rel(base, file)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.ToSlash(relative), ".yaml"), ".yml")
		if name == stack {
			selected = append(selected, file)
		}
	}
	if len(selected) > 0 {
		return selected
	}
	return files
}
