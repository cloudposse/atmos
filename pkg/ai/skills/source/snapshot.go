package source

import (
	"path/filepath"
	"strings"
)

// generatedSourcePaths keeps a source rooted at the project from snapshotting
// its own resolution and installation output. Authored files (including hidden
// client configuration) remain part of the snapshot unless they are recorded
// installation destinations owned by this project.
func (e *Engine) generatedSourcePaths(states map[string]*State) []string {
	paths := []string{filepath.Join(e.Project, ".atmos"), filepath.Join(e.Project, "skills.lock.yaml")}
	for _, state := range states {
		for _, record := range state.Records {
			if record.Project == e.Project {
				paths = append(paths, record.Path)
			}
		}
	}
	return paths
}

func relativeExclusions(root string, paths []string) []string {
	result := []string{}
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		result = append(result, filepath.ToSlash(rel))
	}
	return result
}
