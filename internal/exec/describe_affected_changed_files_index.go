package exec

import (
	"path/filepath"
	"strings"
	"sync"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// changedFilesIndex provides efficient lookup of changed files by base path.
// This reduces PathMatch operations from O(stacks × components × files) to O(stacks × components × relevant_files).
// Expected impact: 60-80% reduction in PathMatch calls.
type changedFilesIndex struct {
	// filesByBasePath maps component base paths to changed files in that path.
	filesByBasePath map[string][]string

	// allFiles contains all changed files for fallback scenarios.
	allFiles []string

	mu sync.RWMutex
}

// newChangedFilesIndex creates an index of changed files organized by base path.
// This enables efficient filtering of relevant files for each component type.
func newChangedFilesIndex(atmosConfig *schema.AtmosConfiguration, changedFiles []string) *changedFilesIndex {
	defer perf.Track(atmosConfig, "exec.newChangedFilesIndex")()

	index := &changedFilesIndex{
		filesByBasePath: make(map[string][]string),
		allFiles:        changedFiles,
	}

	// Pre-compute absolute base paths for each component type.
	basePaths := []string{
		filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath),
		filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath),
		filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath),
		filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath),
	}

	// Convert to absolute paths and normalize.
	normalizedBasePaths := make([]string, 0, len(basePaths))
	for _, basePath := range basePaths {
		absPath, err := filepath.Abs(basePath)
		if err != nil {
			// If conversion fails, use original path.
			absPath = basePath
		}
		normalizedBasePaths = append(normalizedBasePaths, absPath)
		// Initialize empty slice for this base path.
		index.filesByBasePath[absPath] = make([]string, 0)
	}

	// Index each changed file by its base path.
	for _, changedFile := range changedFiles {
		absFile, err := filepath.Abs(changedFile)
		if err != nil {
			// If conversion fails, try using original path.
			absFile = changedFile
		}

		// Find which base path this file belongs to.
		matched := false
		for _, basePath := range normalizedBasePaths {
			if strings.HasPrefix(absFile, basePath) {
				index.filesByBasePath[basePath] = append(index.filesByBasePath[basePath], absFile)
				matched = true
				break
			}
		}

		// If file doesn't match any base path, add to all paths (fallback).
		if !matched {
			for _, basePath := range normalizedBasePaths {
				index.filesByBasePath[basePath] = append(index.filesByBasePath[basePath], absFile)
			}
		}
	}

	return index
}

// getRelevantFiles returns changed files relevant to a specific component.
// This significantly reduces the number of PathMatch operations needed.
func (idx *changedFilesIndex) getRelevantFiles(componentType string, atmosConfig *schema.AtmosConfiguration) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var basePath string

	switch componentType {
	case cfg.TerraformComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath)
	case cfg.HelmfileComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath)
	case cfg.PackerComponentType:
		basePath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath)
	default:
		// Unknown component type - return all files as fallback.
		return idx.allFiles
	}

	absBasePath, err := filepath.Abs(basePath)
	if err != nil {
		// If conversion fails, return all files as fallback.
		return idx.allFiles
	}

	if files, ok := idx.filesByBasePath[absBasePath]; ok {
		return files
	}

	// If base path not found in index, return all files as fallback.
	return idx.allFiles
}

// getAllFiles returns all changed files (for operations that need to check everything).
func (idx *changedFilesIndex) getAllFiles() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.allFiles
}
