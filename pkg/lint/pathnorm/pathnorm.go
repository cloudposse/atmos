// Package pathnorm provides a shared path-normalization utility for the
// Atmos lint subsystem. It defines the single canonical form used by both
// the LintStacks executor (internal/exec) and the L-07 orphan-detection rule
// when comparing import references to physical file paths.
//
// Using one shared implementation avoids the subtle drift that occurred when
// internal/exec.rulesRelNorm and pkg/lint/rules.relNorm were maintained
// independently.
package pathnorm

import (
	"path/filepath"
	"strings"
)

// NormalizeRelNoExt returns the normalised relative-stem form of path for
// consistent cross-platform comparison within the lint engine.
//
// Behaviour:
//   - Absolute paths are first made relative to basePath via filepath.Rel.
//     When basePath is "" the path is left as-is.
//   - YAML extensions (.yaml, .yml) are stripped so that import references that
//     omit extensions match their corresponding physical file names.
//   - The result is passed through filepath.Clean and filepath.ToSlash so that
//     path separators are uniform (/) regardless of the host OS.
func NormalizeRelNoExt(path, basePath string) string {
	if filepath.IsAbs(path) && basePath != "" {
		if rel, err := filepath.Rel(basePath, path); err == nil {
			path = rel
		}
	}
	// Strip YAML extension.
	for _, ext := range []string{".yaml", ".yml"} {
		if len(path) > len(ext) && strings.HasSuffix(path, ext) {
			path = path[:len(path)-len(ext)]
			break
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}
